--- Kanban board operations
--- @rate_limit 0
function call(request)
    local action = request.params.action

    if action == "get_board" then
        return get_board()
    elseif action == "add_card" then
        return add_card(request.params)
    elseif action == "update_card" then
        return update_card(request.params)
    elseif action == "move_card" then
        return move_card(request.params)
    elseif action == "delete_card" then
        return delete_card(request.params)
    elseif action == "add_column" then
        return add_column(request.params)
    elseif action == "update_column" then
        return update_column(request.params)
    elseif action == "delete_column" then
        return delete_column(request.params)
    elseif action == "move_column" then
        return move_column(request.params)
    elseif action == "reorder_columns" then
        return reorder_columns(request.params)
    elseif action == "get_config" then
        return get_config()
    elseif action == "save_config" then
        return save_config(request.params)
    elseif action == "request_join" then
        return request_join(request.params)
    elseif action == "get_my_request" then
        return get_my_request()
    elseif action == "get_requests" then
        return get_requests()
    elseif action == "dismiss_request" then
        return dismiss_request(request.params)
    elseif action == "approve_request" then
        return approve_request(request.params)
    else
        error("unknown action: " .. tostring(action))
    end
end

function get_board()
    local columns = goop.db.query(
        "SELECT _id, name, position, color FROM columns ORDER BY position ASC"
    ) or {}

    local cards = goop.db.query(
        "SELECT _id, column_id, title, description, position, color, assignee, due_date, _owner, _created_at, created_by, moved_by FROM cards ORDER BY position ASC"
    ) or {}

    -- Group cards by column
    local cards_by_column = {}
    for _, card in ipairs(cards) do
        local col_id = card.column_id
        if not cards_by_column[col_id] then
            cards_by_column[col_id] = {}
        end
        table.insert(cards_by_column[col_id], card)
    end

    -- Attach cards to columns
    for _, col in ipairs(columns) do
        col.cards = cards_by_column[col._id] or {}
    end

    return { columns = columns }
end

function add_card(params)
    local column_id = params.column_id
    local title = params.title

    if not column_id or not title or title == "" then
        return { error = "column_id and title required" }
    end

    -- Get max position in column
    local max_pos = goop.db.scalar(
        "SELECT COALESCE(MAX(position), -1) FROM cards WHERE column_id = ?",
        column_id
    ) or -1

    local creator = params.peer_name or ""

    goop.db.exec(
        "INSERT INTO cards (_owner, column_id, title, description, position, color, assignee, due_date, created_by, moved_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
        goop.peer.id,
        column_id,
        title,
        params.description or "",
        max_pos + 1,
        params.color or "",
        params.assignee or "",
        params.due_date or "",
        creator,
        ""
    )

    local id = goop.db.scalar("SELECT MAX(_id) FROM cards WHERE _owner = ?", goop.peer.id)

    return { card_id = id, status = "created" }
end

function update_card(params)
    local card_id = params.card_id

    if not card_id then
        return { error = "card_id required" }
    end

    -- Build update query dynamically
    local updates = {}
    local args = {}

    if params.title then
        table.insert(updates, "title = ?")
        table.insert(args, params.title)
    end
    if params.description ~= nil then
        table.insert(updates, "description = ?")
        table.insert(args, params.description)
    end
    if params.color ~= nil then
        table.insert(updates, "color = ?")
        table.insert(args, params.color)
    end
    if params.assignee ~= nil then
        table.insert(updates, "assignee = ?")
        table.insert(args, params.assignee)
    end
    if params.due_date ~= nil then
        table.insert(updates, "due_date = ?")
        table.insert(args, params.due_date)
    end

    if #updates == 0 then
        return { error = "nothing to update" }
    end

    table.insert(updates, "_updated_at = CURRENT_TIMESTAMP")
    table.insert(args, card_id)

    local query = "UPDATE cards SET " .. table.concat(updates, ", ") .. " WHERE _id = ?"
    goop.db.exec(query, table.unpack(args))

    return { status = "updated" }
end

function move_card(params)
    local card_id = params.card_id
    local to_column = params.to_column
    local to_position = params.to_position

    if not card_id or not to_column then
        return { error = "card_id and to_column required" }
    end

    -- If no position specified, add to end
    if not to_position then
        to_position = goop.db.scalar(
            "SELECT COALESCE(MAX(position), -1) + 1 FROM cards WHERE column_id = ?",
            to_column
        ) or 0
    end

    -- Shift cards down in target column
    goop.db.exec(
        "UPDATE cards SET position = position + 1 WHERE column_id = ? AND position >= ?",
        to_column, to_position
    )

    -- Move the card and record who moved it
    local mover = params.peer_name or ""
    goop.db.exec(
        "UPDATE cards SET column_id = ?, position = ?, moved_by = ?, _updated_at = CURRENT_TIMESTAMP WHERE _id = ?",
        to_column, to_position, mover, card_id
    )

    return { status = "moved" }
end

function delete_card(params)
    local card_id = params.card_id

    if not card_id then
        return { error = "card_id required" }
    end

    -- Only owner or site owner can delete
    local card = goop.db.query("SELECT _owner FROM cards WHERE _id = ?", card_id)
    if not card or #card == 0 then
        return { error = "card not found" }
    end

    if card[1]._owner ~= goop.peer.id and goop.peer.id ~= goop.self.id then
        return { error = "not authorized to delete this card" }
    end

    goop.db.exec("DELETE FROM cards WHERE _id = ?", card_id)

    return { status = "deleted" }
end

-- Column management (owner only)
function add_column(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can add columns" }
    end

    local name = params.name
    if not name or name == "" then
        return { error = "column name required" }
    end

    local max_pos = goop.db.scalar("SELECT COALESCE(MAX(position), -1) FROM columns") or -1

    goop.db.exec(
        "INSERT INTO columns (_owner, name, position, color) VALUES (?, ?, ?, ?)",
        goop.self.id,
        name,
        max_pos + 1,
        params.color or "#5b6abf"
    )

    local id = goop.db.scalar("SELECT MAX(_id) FROM columns")

    return { column_id = id, status = "created" }
end

function update_column(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can update columns" }
    end

    local column_id = params.column_id
    if not column_id then
        return { error = "column_id required" }
    end

    local updates = {}
    local args = {}

    if params.name then
        table.insert(updates, "name = ?")
        table.insert(args, params.name)
    end
    if params.color then
        table.insert(updates, "color = ?")
        table.insert(args, params.color)
    end

    if #updates == 0 then
        return { error = "nothing to update" }
    end

    table.insert(updates, "_updated_at = CURRENT_TIMESTAMP")
    table.insert(args, column_id)

    local query = "UPDATE columns SET " .. table.concat(updates, ", ") .. " WHERE _id = ?"
    goop.db.exec(query, table.unpack(args))

    return { status = "updated" }
end

function delete_column(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can delete columns" }
    end

    local column_id = params.column_id
    if not column_id then
        return { error = "column_id required" }
    end

    -- Check if column has cards
    local card_count = goop.db.scalar("SELECT COUNT(*) FROM cards WHERE column_id = ?", column_id)
    if card_count and card_count > 0 then
        return { error = "cannot delete column with cards" }
    end

    goop.db.exec("DELETE FROM columns WHERE _id = ?", column_id)

    return { status = "deleted" }
end

function move_column(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can reorder columns" }
    end

    local column_id = params.column_id
    local direction = params.direction

    if not column_id or not direction then
        return { error = "column_id and direction required" }
    end

    -- Get current column's position
    local rows = goop.db.query("SELECT _id, position FROM columns WHERE _id = ?", column_id)
    if not rows or #rows == 0 then
        return { error = "column not found" }
    end
    local current_pos = rows[1].position

    -- Find adjacent column
    local adjacent
    if direction == "left" then
        adjacent = goop.db.query(
            "SELECT _id, position FROM columns WHERE position < ? ORDER BY position DESC LIMIT 1",
            current_pos
        )
    elseif direction == "right" then
        adjacent = goop.db.query(
            "SELECT _id, position FROM columns WHERE position > ? ORDER BY position ASC LIMIT 1",
            current_pos
        )
    else
        return { error = "direction must be 'left' or 'right'" }
    end

    if not adjacent or #adjacent == 0 then
        return { error = "cannot move further in that direction" }
    end

    local adj_id = adjacent[1]._id
    local adj_pos = adjacent[1].position

    -- Swap positions
    goop.db.exec("UPDATE columns SET position = ?, _updated_at = CURRENT_TIMESTAMP WHERE _id = ?", adj_pos, column_id)
    goop.db.exec("UPDATE columns SET position = ?, _updated_at = CURRENT_TIMESTAMP WHERE _id = ?", current_pos, adj_id)

    return { status = "moved" }
end

function reorder_columns(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can reorder columns" }
    end
    local ids = params.column_ids
    if not ids then
        return { error = "column_ids required" }
    end
    for i, id in ipairs(ids) do
        goop.db.exec("UPDATE columns SET position = ?, _updated_at = CURRENT_TIMESTAMP WHERE _id = ?", i - 1, id)
    end
    return { status = "reordered" }
end

function get_config()
    local rows = goop.db.query("SELECT key, value FROM kanban_config")
    if not rows or #rows == 0 then
        return { title = "Kanban Board", subtitle = "Shared team kanban board" }
    end
    local cfg = {}
    for _, row in ipairs(rows) do
        cfg[row.key] = row.value
    end
    return cfg
end

function save_config(params)
    if goop.peer.id ~= goop.self.id then
        error("Only the site owner can change board settings")
    end

    local keys = { "title", "subtitle" }
    for _, key in ipairs(keys) do
        if params[key] then
            goop.db.exec(
                "INSERT INTO kanban_config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
                key, params[key]
            )
        end
    end

    return { status = "saved" }
end

-- Join request actions

function request_join(params)
    -- Check if already requested
    local existing = goop.db.query(
        "SELECT status FROM join_requests WHERE _owner = ?",
        goop.peer.id
    )
    if existing and #existing > 0 then
        local st = existing[1].status
        if st == "pending" then
            return { status = "pending" }
        end
        -- Approved or dismissed: clear old request so they can re-request
        goop.db.exec("DELETE FROM join_requests WHERE _owner = ?", goop.peer.id)
    end

    -- Insert new request (Lua runs as owner context due to insert_policy: "owner")
    goop.db.exec(
        "INSERT INTO join_requests (_owner, peer_name, message, status) VALUES (?, ?, ?, 'pending')",
        goop.peer.id,
        params.peer_name or "",
        params.message or ""
    )

    return { status = "pending" }
end

function get_my_request()
    local rows = goop.db.query(
        "SELECT status FROM join_requests WHERE _owner = ?",
        goop.peer.id
    )
    if not rows or #rows == 0 then
        return { status = "none" }
    end
    return { status = rows[1].status }
end

function get_requests()
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can view requests" }
    end

    local rows = goop.db.query(
        "SELECT _id, _owner, peer_name, message, status, _created_at FROM join_requests WHERE status = 'pending' ORDER BY _created_at ASC"
    ) or {}

    return { requests = rows }
end

function dismiss_request(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can dismiss requests" }
    end

    if not params.request_id then
        return { error = "request_id required" }
    end

    goop.db.exec(
        "UPDATE join_requests SET status = 'dismissed', _updated_at = CURRENT_TIMESTAMP WHERE _id = ?",
        params.request_id
    )

    return { status = "dismissed" }
end

function approve_request(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can approve requests" }
    end

    if not params.request_id then
        return { error = "request_id required" }
    end

    -- Get the peer_id (_owner) for JS to use for group invite
    local rows = goop.db.query(
        "SELECT _owner FROM join_requests WHERE _id = ?",
        params.request_id
    )
    if not rows or #rows == 0 then
        return { error = "request not found" }
    end

    goop.db.exec(
        "UPDATE join_requests SET status = 'approved', _updated_at = CURRENT_TIMESTAMP WHERE _id = ?",
        params.request_id
    )

    return { status = "approved", peer_id = rows[1]._owner }
end

function handle(args)
    return "Visit my site to use the kanban board!"
end
