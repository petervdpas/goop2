--- Todo list operations
--- @rate_limit 0

local todos = nil

local function init()
    if not todos then todos = goop.orm("todos") end
end

local function list_todos()
    return { todos = todos:find({ order = "position ASC" }) or {} }
end

local function add_todo(params)
    if not params.text or params.text == "" then
        return { error = "text required" }
    end
    local max = todos:aggregate("COALESCE(MAX(position), -1) as v")
    local pos = (max and #max > 0) and max[1].v + 1 or 0
    local id = todos:insert({
        text = params.text,
        done = 0,
        position = pos,
        created_by = params.peer_name or "",
    })
    return { id = id }
end

local function toggle_todo(params)
    if not params.id then return { error = "id required" } end
    local row = todos:find_one({ where = "_id = ?", args = { params.id }, fields = { "done" } })
    if not row then return { error = "not found" } end
    todos:update(params.id, { done = row.done == 0 and 1 or 0 })
    return { status = "toggled" }
end

local function delete_todo(params)
    if not params.id then return { error = "id required" } end
    todos:delete(params.id)
    return { status = "deleted" }
end

local function reorder_todos(params)
    if not params.ids then return { error = "ids required" } end
    for i, id in ipairs(params.ids) do
        todos:update(id, { position = i - 1 })
    end
    return { status = "reordered" }
end

local function i(fn) return function(p) init(); return fn(p) end end

local dispatch = goop.route({
    list    = i(list_todos),
    add     = i(add_todo),
    toggle  = i(toggle_todo),
    delete  = goop.owner(i(delete_todo)),
    reorder = goop.owner(i(reorder_todos)),
})

function call(req) return dispatch(req) end

function handle(args)
    return "Visit my site to see the todo list!"
end
