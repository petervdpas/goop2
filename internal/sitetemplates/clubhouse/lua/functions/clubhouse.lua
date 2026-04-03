--- Clubhouse room management
--- @rate_limit 0

local rooms = nil
local function init() if not rooms then rooms = goop.orm("rooms") end end

local dispatch = goop.route({
    rooms = function()
        init()
        local rows = rooms:find({ where = "status = 'open'", order = "_id DESC", limit = 50 }) or {}
        local active = {}
        for _, r in ipairs(goop.chat.rooms()) do
            active[r.id] = true
        end
        local result = {}
        for _, r in ipairs(rows) do
            if active[r.group_id] then
                result[#result + 1] = {
                    _id = r._id,
                    name = r.name,
                    description = r.description,
                    group_id = r.group_id,
                    max_members = r.max_members,
                    status = r.status,
                }
            else
                rooms:update(r._id, { status = "closed" })
            end
        end
        return { rooms = result }
    end,

    create = goop.owner(function(p)
        init()
        local name = p.name or ""
        if name == "" then error("room name required") end

        local room, err = goop.chat.create(name, p.description or "", tonumber(p.max_members) or 0, goop.template.name)
        if err then error(err) end

        local id = rooms:insert({
            name = name,
            description = p.description or "",
            group_id = room.id,
            max_members = tonumber(p.max_members) or 0,
            status = "open",
        })

        return { room_id = id, group_id = room.id }
    end),

    close = goop.owner(function(p)
        init()
        local room_id = tonumber(p.room_id)
        if not room_id then error("room_id required") end
        local room = rooms:get(room_id)
        if not room then error("room not found") end

        local err = goop.chat.close(room.group_id)
        if err then error(err) end
        rooms:update(room_id, { status = "closed" })
        return { ok = true }
    end),
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
