--- Test: goop.route() dispatcher and goop.owner() wrapper
--- @rate_limit 0

local items = nil

local function do_list(params)
    if not items then items = goop.orm("items") end
    return items:find({ order = "_id ASC" })
end

local function do_insert(params)
    if not items then items = goop.orm("items") end
    local id = items:insert({ name = params.name })
    return { id = id }
end

local function do_count(params)
    if not items then items = goop.orm("items") end
    return { n = items:count() }
end

local function do_delete(params)
    if not items then items = goop.orm("items") end
    items:delete(params.id)
    return { ok = true }
end

local function do_admin_only(params)
    return { secret = "admin-data", peer = goop.peer.id }
end

local dispatch = goop.route({
    list = do_list,
    insert = do_insert,
    count = do_count,
    delete = do_delete,
    admin = goop.owner(do_admin_only),
})

function call(req)
    return dispatch(req)
end
