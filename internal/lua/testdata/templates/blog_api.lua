--- Test: Blog-like pattern (posts CRUD + config via goop.config)
--- @rate_limit 0

local posts = nil
local cfg = nil

function call(req)
    if not posts then posts = goop.orm("posts") end
    if not cfg then cfg = goop.config("blog_config", {
        title = "My Blog",
        theme = "light",
    }) end

    local action = req.params.action

    if action == "page" then
        local rows = posts:find({
            where = "published = 1",
            order = "_id DESC",
            limit = 50,
        })
        return {
            posts = rows or {},
            config = { title = cfg.title, theme = cfg.theme },
            can_write = goop.peer.id == goop.self.id,
        }

    elseif action == "save_post" then
        local p = req.params
        if not p.title or p.title == "" then error("title required") end
        if not p.body or p.body == "" then error("body required") end

        local slug = string.lower(p.title)
        slug = string.gsub(slug, "[^a-z0-9]+", "-")
        slug = string.gsub(slug, "^%-+", "")
        slug = string.gsub(slug, "%-+$", "")

        local id = tonumber(p.id)
        if id and id > 0 then
            posts:update(id, { title = p.title, body = p.body, slug = slug })
            return { id = id }
        else
            local new_id = posts:insert({
                title = p.title, body = p.body, slug = slug,
                author_name = goop.peer.label or "",
                published = 1,
            })
            return { id = new_id }
        end

    elseif action == "delete_post" then
        local id = tonumber(req.params.id)
        if not id then error("id required") end
        posts:delete(id)
        return { ok = true }

    elseif action == "get_post" then
        local row = posts:find_one({
            where = "slug = ? AND published = 1",
            args = { req.params.slug },
        })
        if not row then
            return { found = false }
        end
        return { found = true, post = row }

    elseif action == "save_config" then
        cfg:set(req.params.key, req.params.value or "")
        return { ok = true }

    elseif action == "get_config" then
        return { title = cfg.title, theme = cfg.theme }

    else
        error("unknown action: " .. tostring(action))
    end
end
