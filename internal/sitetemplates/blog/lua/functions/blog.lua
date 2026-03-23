--- Blog data service
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

    local rows = goop.db.query(
        "SELECT _id, _owner, title, body, author_name, image, slug, _created_at FROM posts WHERE slug = ? AND published = 1 LIMIT 1",
        slug
    )
    if not rows or #rows == 0 then
        rows = goop.db.query(
            "SELECT _id, _owner, title, body, author_name, image, slug, _created_at FROM posts WHERE _id = ? AND published = 1 LIMIT 1",
            slug
        )
    end
    if not rows or #rows == 0 then
        return { found = false }
    end

    return { found = true, post = rows[1] }
end

function list_posts()
    local rows = goop.db.query(
        "SELECT _id, _owner, title, body, author_name, image, slug, published, _created_at FROM posts WHERE published = 1 ORDER BY _id DESC LIMIT 50"
    )
    return { posts = rows or {} }
end

function get_config()
    local rows = goop.db.query(
        "SELECT key, value FROM blog_config"
    )
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
