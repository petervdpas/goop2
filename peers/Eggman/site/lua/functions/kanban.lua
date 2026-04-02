--- Kanban board operations
--- @rate_limit 0

local columns_tbl = nil
local cards_tbl = nil
local cfg = nil
local requests_tbl = nil

local function init()
    if not columns_tbl then columns_tbl = goop.orm("columns") end
    if not cards_tbl then cards_tbl = goop.orm("cards") end
    if not cfg then cfg = goop.config("kanban_config", { title = "Kanban Board", subtitle = "Shared team kanban board" }) end
    if not requests_tbl then requests_tbl = goop.orm("join_requests") end
end

local function get_board()
    local cols = columns_tbl:find({ order = "position ASC" }) or {}
    local cards = cards_tbl:find({ order = "position ASC" }) or {}

    local cards_by_column = {}
    for _, card in ipairs(cards) do
        local col_id = card.column_id
        if not cards_by_column[col_id] then
            cards_by_column[col_id] = {}
        end
        table.insert(cards_by_column[col_id], card)
    end

    for _, col in ipairs(cols) do
        col.cards = cards_by_column[col._id] or {}
    end

    return { columns = cols }
end

local function add_card(params)
    local column_id = params.column_id
    local title = params.title

    if not column_id or not title or title == "" then
        return { error = "column_id and title required" }
    end

    local max_rows = cards_tbl:aggregate("COALESCE(MAX(position), -1) as max_pos", {
        where = "column_id = ?",
        args = { column_id },
    })
    local max_pos = (max_rows and #max_rows > 0) and max_rows[1].max_pos or -1

    local id = cards_tbl:insert({
        column_id = column_id,
        title = title,
        description = params.description or "",
        position = max_pos + 1,
        color = params.color or "",
        assignee = params.assignee or "",
        due_date = params.due_date or "",
        created_by = params.peer_name or "",
        moved_by = "",
    })

    return { card_id = id, status = "created" }
end

local function update_card(params)
    local card_id = params.card_id
    if not card_id then
        return { error = "card_id required" }
    end

    local data = {}
    if params.title then data.title = params.title end
    if params.description ~= nil then data.description = params.description end
    if params.color ~= nil then data.color = params.color end
    if params.assignee ~= nil then data.assignee = params.assignee end
    if params.due_date ~= nil then data.due_date = params.due_date end

    if not next(data) then
        return { error = "nothing to update" }
    end

    cards_tbl:update(card_id, data)
    return { status = "updated" }
end

local function move_card(params)
    local card_id = params.card_id
    local to_column = params.to_column
    local to_position = params.to_position

    if not card_id or not to_column then
        return { error = "card_id and to_column required" }
    end

    if not to_position then
        local rows = cards_tbl:aggregate("COALESCE(MAX(position), -1) + 1 as next_pos", {
            where = "column_id = ?",
            args = { to_column },
        })
        to_position = (rows and #rows > 0) and rows[1].next_pos or 0
    end

    cards_tbl:update_where(
        { position = goop.expr("position + 1") },
        { where = "column_id = ? AND position >= ?", args = { to_column, to_position } }
    )

    cards_tbl:update(card_id, {
        column_id = to_column,
        position = to_position,
        moved_by = params.peer_name or "",
    })

    return { status = "moved" }
end

local function delete_card(params)
    local card_id = params.card_id
    if not card_id then
        return { error = "card_id required" }
    end

    local card = cards_tbl:find_one({
        where = "_id = ?",
        args = { card_id },
        fields = { "_owner" },
    })
    if not card then
        return { error = "card not found" }
    end

    if card._owner ~= goop.peer.id and goop.peer.id ~= goop.self.id then
        return { error = "not authorized to delete this card" }
    end

    cards_tbl:delete(card_id)
    return { status = "deleted" }
end

local function add_column(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can add columns" }
    end

    local name = params.name
    if not name or name == "" then
        return { error = "column name required" }
    end

    local max_rows = columns_tbl:aggregate("COALESCE(MAX(position), -1) as max_pos")
    local max_pos = (max_rows and #max_rows > 0) and max_rows[1].max_pos or -1

    local id = columns_tbl:insert({
        name = name,
        position = max_pos + 1,
        color = params.color or "#5b6abf",
    })

    return { column_id = id, status = "created" }
end

local function update_column(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can update columns" }
    end

    local column_id = params.column_id
    if not column_id then
        return { error = "column_id required" }
    end

    local data = {}
    if params.name then data.name = params.name end
    if params.color then data.color = params.color end

    if not next(data) then
        return { error = "nothing to update" }
    end

    columns_tbl:update(column_id, data)
    return { status = "updated" }
end

local function delete_column(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can delete columns" }
    end

    local column_id = params.column_id
    if not column_id then
        return { error = "column_id required" }
    end

    local n = cards_tbl:count({ where = "column_id = ?", args = { column_id } })
    if n and n > 0 then
        return { error = "cannot delete column with cards" }
    end

    columns_tbl:delete(column_id)
    return { status = "deleted" }
end

local function move_column(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can reorder columns" }
    end

    local column_id = params.column_id
    local direction = params.direction

    if not column_id or not direction then
        return { error = "column_id and direction required" }
    end

    local col = columns_tbl:find_one({ where = "_id = ?", args = { column_id }, fields = { "_id", "position" } })
    if not col then
        return { error = "column not found" }
    end
    local current_pos = col.position

    local adjacent
    if direction == "left" then
        adjacent = columns_tbl:find_one({
            where = "position < ?",
            args = { current_pos },
            order = "position DESC",
        })
    elseif direction == "right" then
        adjacent = columns_tbl:find_one({
            where = "position > ?",
            args = { current_pos },
            order = "position ASC",
        })
    else
        return { error = "direction must be 'left' or 'right'" }
    end

    if not adjacent then
        return { error = "cannot move further in that direction" }
    end

    columns_tbl:update(column_id, { position = adjacent.position })
    columns_tbl:update(adjacent._id, { position = current_pos })

    return { status = "moved" }
end

local function reorder_columns(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can reorder columns" }
    end
    local ids = params.column_ids
    if not ids then
        return { error = "column_ids required" }
    end
    for i, id in ipairs(ids) do
        columns_tbl:update(id, { position = i - 1 })
    end
    return { status = "reordered" }
end

local function get_config()
    return { title = cfg.title, subtitle = cfg.subtitle }
end

local function save_config(params)
    if goop.peer.id ~= goop.self.id then
        error("Only the site owner can change board settings")
    end

    if params.title then cfg:set("title", params.title) end
    if params.subtitle then cfg:set("subtitle", params.subtitle) end

    return { status = "saved" }
end

local function request_join(params)
    local existing = requests_tbl:find_one({
        where = "_owner = ?",
        args = { goop.peer.id },
        fields = { "status" },
    })
    if existing then
        if existing.status == "pending" then
            return { status = "pending" }
        end
        requests_tbl:delete_where({ where = "_owner = ?", args = { goop.peer.id } })
    end

    requests_tbl:insert({
        peer_name = params.peer_name or "",
        message = params.message or "",
        status = "pending",
    })

    return { status = "pending" }
end

local function get_my_request()
    local row = requests_tbl:find_one({
        where = "_owner = ?",
        args = { goop.peer.id },
        fields = { "status" },
    })
    if not row then
        return { status = "none" }
    end
    return { status = row.status }
end

local function get_requests()
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can view requests" }
    end

    local rows = requests_tbl:find({
        where = "status = 'pending'",
        order = "_created_at ASC",
    })

    return { requests = rows or {} }
end

local function dismiss_request(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can dismiss requests" }
    end
    if not params.request_id then
        return { error = "request_id required" }
    end

    requests_tbl:update(params.request_id, { status = "dismissed" })
    return { status = "dismissed" }
end

local function approve_request(params)
    if goop.peer.id ~= goop.self.id then
        return { error = "only the site owner can approve requests" }
    end
    if not params.request_id then
        return { error = "request_id required" }
    end

    local row = requests_tbl:find_one({
        where = "_id = ?",
        args = { params.request_id },
        fields = { "_owner" },
    })
    if not row then
        return { error = "request not found" }
    end

    requests_tbl:update(params.request_id, { status = "approved" })
    return { status = "approved", peer_id = row._owner }
end

local function i(fn) return function(p) init(); return fn(p) end end

local dispatch = goop.route({
    get_board      = i(get_board),
    add_card       = i(add_card),
    update_card    = i(update_card),
    move_card      = i(move_card),
    delete_card    = i(delete_card),
    add_column     = goop.owner(i(add_column)),
    update_column  = goop.owner(i(update_column)),
    delete_column  = goop.owner(i(delete_column)),
    move_column    = goop.owner(i(move_column)),
    reorder_columns = goop.owner(i(reorder_columns)),
    get_config     = i(get_config),
    save_config    = goop.owner(i(save_config)),
    request_join   = i(request_join),
    get_my_request = i(get_my_request),
    get_requests   = goop.owner(i(get_requests)),
    dismiss_request = goop.owner(i(dismiss_request)),
    approve_request = goop.owner(i(approve_request)),
})

function call(req) return dispatch(req) end

function handle(args)
    return "Visit my site to use the kanban board!"
end
