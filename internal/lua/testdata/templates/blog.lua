--- Blog data service — ORM DSL via goop.orm
--- @rate_limit 0

local posts = nil
local config = nil

function call(request)
    if not posts then posts = goop.orm("posts") end
    if not config then config = goop.orm("blog_config") end

    local action = request.params.action

    if action == "page" then
        return page()
    elseif action == "get_post" then
        return get_post(request.params.slug)
    elseif action == "list_posts" then
        return list_posts()
    elseif action == "get_config" then
        return get_config()
    elseif action == "save_config" then
        return save_config(request.params.key, request.params.value)
    elseif action == "save_post" then
        return save_post(request.params)
    elseif action == "delete_post" then
        return delete_post(request.params.id)
    else
        error("unknown action: " .. tostring(action))
    end
end

function page()
    local is_owner = goop.peer.id == goop.self.id
    local is_group = false

    return {
        posts = list_posts().posts,
        config = get_config(),
        can_write = is_owner or is_group,
        can_admin = is_owner,
    }
end

function get_post(slug)
    if not slug or slug == "" then
        return { found = false }
    end

    local row = posts:find_one({
        where = "slug = ? AND published = 1",
        args = { slug },
        fields = { "_id", "_owner", "title", "body", "author_name", "image", "slug", "_created_at" },
    })
    if not row then
        row = posts:find_one({
            where = "_id = ? AND published = 1",
            args = { slug },
            fields = { "_id", "_owner", "title", "body", "author_name", "image", "slug", "_created_at" },
        })
    end
    if not row then
        return { found = false }
    end

    return { found = true, post = row }
end

function list_posts()
    local rows = posts:find({
        where = "published = 1",
        fields = { "_id", "_owner", "title", "body", "author_name", "image", "slug", "published", "_created_at" },
        order = "_id DESC",
        limit = 50,
    })
    return { posts = rows or {} }
end

function get_config()
    local rows = config:find({
        fields = { "key", "value" },
    })
    local result = {}
    if rows then
        for _, r in ipairs(rows) do
            result[r.key] = r.value
        end
    end
    return result
end

function save_config(key, value)
    if not key or key == "" then error("key required") end
    config:upsert("key", { key = key, value = value or "" })
    return { ok = true }
end

function slugify(title)
    local s = string.lower(title)
    s = string.gsub(s, "[^a-z0-9]+", "-")
    s = string.gsub(s, "^%-+", "")
    s = string.gsub(s, "%-+$", "")
    return s
end

function save_post(p)
    local title = p.title or ""
    local body = p.body or ""
    if title == "" or body == "" then error("title and body required") end

    local slug = slugify(title)
    local id = tonumber(p.id)

    if id and id > 0 then
        posts:update(id, {
            title = title,
            body = body,
            slug = slug,
            image = p.image or "",
        })
        return { id = id }
    else
        local new_id = posts:insert({
            title = title,
            body = body,
            slug = slug,
            author_name = goop.peer.label or "",
            image = p.image or "",
            published = 1,
        })
        return { id = new_id }
    end
end

function delete_post(id)
    id = tonumber(id)
    if not id then error("id required") end

    local row = posts:get(id)
    if not row then error("post not found") end

    posts:delete(id)
    return { ok = true, image = row.image or "" }
end

function handle(args)
    return "Visit my site to read my blog!"
end
