--- Clubhouse room management
--- @rate_limit 0

local rooms = nil

local function init()
    if not rooms then rooms = goop.orm("rooms") end
end

local function list_rooms()
    init()
    local rows = rooms:find({ where = "status = 'open'", order = "_id DESC", limit = 50 }) or {}
    local hosted = {}
    local groups = goop.group.list()
    for _, g in ipairs(groups) do
        hosted[g.id] = true
    end
    local result = {}
    for _, r in ipairs(rows) do
        if hosted[r.group_id] then
            local members = goop.group.members(r.group_id)
            result[#result + 1] = {
                _id = r._id,
                name = r.name,
                description = r.description,
                group_id = r.group_id,
                max_members = r.max_members,
                status = r.status,
                member_count = #members,
            }
        else
            rooms:update(r._id, { status = "closed" })
        end
    end
    return { rooms = result }
end

local function create_room(p)
    init()
    local name = p.name or ""
    if name == "" then error("room name required") end
    local desc = p.description or ""
    local max = tonumber(p.max_members) or 0

    local group_id = goop.group.create(name, "clubhouse", max)

    local id = rooms:insert({
        name = name,
        description = desc,
        group_id = group_id,
        max_members = max,
        status = "open",
    })

    return { room_id = id, group_id = group_id }
end

local function join_room(p)
    init()
    local room_id = tonumber(p.room_id)
    if not room_id then error("room_id required") end

    local room = rooms:get(room_id)
    if not room then error("room not found") end
    if room.status ~= "open" then error("room is closed") end

    local members = goop.group.members(room.group_id)
    if room.max_members > 0 and #members >= room.max_members then
        error("room is full")
    end

    if goop.peer.id ~= goop.self.id then
        goop.group.add(room.group_id, goop.peer.id)
    end

    return {
        group_id = room.group_id,
        name = room.name,
        description = room.description,
    }
end

local function leave_room(p)
    init()
    local room_id = tonumber(p.room_id)
    if not room_id then error("room_id required") end

    local room = rooms:get(room_id)
    if not room then error("room not found") end

    goop.group.remove(room.group_id, goop.peer.id)
    return { ok = true }
end

local function close_room(p)
    init()
    local room_id = tonumber(p.room_id)
    if not room_id then error("room_id required") end

    local room = rooms:get(room_id)
    if not room then error("room not found") end

    pcall(function() goop.group.close(room.group_id) end)
    rooms:update(room_id, { status = "closed" })
    return { ok = true }
end

local function room_members(p)
    init()
    local room_id = tonumber(p.room_id)
    if not room_id then error("room_id required") end

    local room = rooms:get(room_id)
    if not room then error("room not found") end

    return { members = goop.group.members(room.group_id) }
end

local function send_message(p)
    init()
    local room_id = tonumber(p.room_id)
    if not room_id then error("room_id required") end

    local room = rooms:get(room_id)
    if not room then error("room not found") end

    goop.group.send(room.group_id, {
        type = "chat",
        text = p.text or "",
        from = goop.peer.id,
        label = goop.peer.label or "",
    })
    return { ok = true }
end

local dispatch = goop.route({
    rooms        = list_rooms,
    create       = goop.owner(create_room),
    join         = join_room,
    leave        = leave_room,
    close        = goop.owner(close_room),
    members      = room_members,
    send_message = send_message,
})

function call(req) return dispatch(req) end

function on_group_close(group_id)
    init()
    local rows = rooms:find({ where = "group_id = ? AND status = 'open'", args = { group_id } }) or {}
    for _, r in ipairs(rows) do
        rooms:update(r._id, { status = "closed" })
    end
end

function handle(args)
    return "Visit my site to join a chat room!"
end
