--- Test: goop.expr() SQL expressions in update_where
--- @rate_limit 0

local items = nil
local function init() if not items then items = goop.orm("items") end end

local dispatch = goop.route({
    seed = function()
        init()
        items:insert({ name = "a", priority = 5 })
        items:insert({ name = "b", priority = 10 })
        items:insert({ name = "c", priority = 3 })
        return { ok = true }
    end,

    increment_all = function(p)
        init()
        return items:update_where(
            { priority = goop.expr("priority + " .. (p.amount or 1)) },
            { where = "1=1" }
        )
    end,

    increment_where = function(p)
        init()
        return items:update_where(
            { priority = goop.expr("priority + " .. (p.amount or 1)) },
            { where = "name = ?", args = { p.name } }
        )
    end,

    decrement = function(p)
        init()
        return items:update_where(
            { priority = goop.expr("priority - 1") },
            { where = "priority > ?", args = { p.min or 0 } }
        )
    end,

    get = function(p)
        init()
        local row = items:get_by("name", p.name)
        return row or { error = "not found" }
    end,

    list = function()
        init()
        return items:find({ order = "name ASC" })
    end,
})

function call(req) return dispatch(req) end
