--- Test: Blog-like pattern (posts CRUD + config via goop.config + goop.route)
--- @rate_limit 0

local posts = nil
local cfg = nil

local function init()
    if not posts then posts = goop.orm("posts") end
    if not cfg then cfg = goop.config("blog_config", {
        title = "My Blog", theme = "light",
    }) end
end

local function slugify(title)
    local s = string.lower(title)
    s = string.gsub(s, "[^a-z0-9]+", "-")
    s = string.gsub(s, "^%-+", "")
    s = string.gsub(s, "%-+$", "")
    return s
end

local dispatch = goop.route({
    page = function()
        init()
        return {
            posts = posts:find({ where = "published = 1", order = "_id DESC", limit = 50 }) or {},
            config = { title = cfg.title, theme = cfg.theme },
            can_write = goop.peer.id == goop.self.id,
        }
    end,

    save_post = function(p)
        init()
        if not p.title or p.title == "" then error("title required") end
        if not p.body or p.body == "" then error("body required") end
        local slug = slugify(p.title)
        local id = tonumber(p.id)
        if id and id > 0 then
            posts:update(id, { title = p.title, body = p.body, slug = slug })
            return { id = id }
        else
            return { id = posts:insert({
                title = p.title, body = p.body, slug = slug,
                author_name = goop.peer.label or "", published = 1,
            }) }
        end
    end,

    delete_post = function(p)
        init()
        local id = tonumber(p.id)
        if not id then error("id required") end
        posts:delete(id)
        return { ok = true }
    end,

    get_post = function(p)
        init()
        local slug = p.slug
        if not slug or slug == "" then return { found = false } end
        local row = posts:find_one({ where = "slug = ? AND published = 1", args = { slug } })
        if not row then return { found = false } end
        return { found = true, post = row }
    end,

    save_config = goop.owner(function(p)
        init()
        cfg:set(p.key, p.value or "")
        return { ok = true }
    end),

    get_config = function()
        init()
        return { title = cfg.title, theme = cfg.theme }
    end,
})

function call(req) return dispatch(req) end
