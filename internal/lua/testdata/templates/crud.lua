--- Test: ORM DSL CRUD operations
--- @rate_limit 0

local items = nil

function call(req)
    if not items then items = goop.orm("items") end
    local action = req.params.action

    if action == "insert" then
        local id = items:insert({ name = req.params.name, priority = req.params.priority or 0 })
        return { id = id }

    elseif action == "get" then
        local row = items:get(req.params.id)
        return row or { error = "not found" }

    elseif action == "get_by" then
        local row = items:get_by("name", req.params.name)
        return row or { error = "not found" }

    elseif action == "find" then
        return items:find(req.params.opts or {})

    elseif action == "find_one" then
        local row = items:find_one(req.params.opts or {})
        return row or { error = "not found" }

    elseif action == "list" then
        return items:list(req.params.limit or 100)

    elseif action == "count" then
        return { n = items:count(req.params.opts) }

    elseif action == "exists" then
        return { yes = items:exists(req.params.opts) }

    elseif action == "pluck" then
        return items:pluck(req.params.column, req.params.opts)

    elseif action == "update" then
        items:update(req.params.id, req.params.data)
        return { ok = true }

    elseif action == "delete" then
        items:delete(req.params.id)
        return { ok = true }

    elseif action == "upsert" then
        return items:upsert(req.params.key_col, req.params.data)

    elseif action == "seed" then
        return { n = items:seed(req.params.rows) }

    elseif action == "validate" then
        local ok, err = items:validate(req.params.data)
        return { valid = ok, error = err }

    elseif action == "schema" then
        return {
            name = items.name,
            system_key = items.system_key,
            col_count = #items.columns,
            insert_policy = items.access and items.access.insert or "none",
        }

    else
        error("unknown action: " .. tostring(action))
    end
end
