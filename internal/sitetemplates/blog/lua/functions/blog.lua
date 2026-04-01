--- Blog data service — ORM queries via goop.schema
--- @rate_limit 0
function call(request)
    local action = request.params.action

    if action == "get_post" then
        return get_post(request.params.slug)
    elseif action == "list_posts" then
        return list_posts()
    elseif action == "get_config" then
        return get_config()
    else
        error("unknown action: " .. tostring(action))
    end
end

function get_post(slug)
    if not slug or slug == "" then
        return { found = false }
    end

    local row = goop.schema.find_one("posts", {
        where = "slug = ? AND published = 1",
        args = { slug },
        fields = { "_id", "_owner", "title", "body", "author_name", "image", "slug", "_created_at" },
    })
    if not row then
        row = goop.schema.find_one("posts", {
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
    local rows = goop.schema.find("posts", {
        where = "published = 1",
        fields = { "_id", "_owner", "title", "body", "author_name", "image", "slug", "published", "_created_at" },
        order = "_id DESC",
        limit = 50,
    })
    return { posts = rows or {} }
end

function get_config()
    local rows = goop.schema.find("blog_config", {
        fields = { "key", "value" },
    })
    local config = {}
    if rows then
        for _, r in ipairs(rows) do
            config[r.key] = r.value
        end
    end
    return config
end

function handle(args)
    return "Visit my site to read my blog!"
end
