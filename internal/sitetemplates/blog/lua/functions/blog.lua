--- Blog data service
--- @rate_limit 0

local posts = nil
local cfg = nil

local function init()
    if not posts then posts = goop.orm("posts") end
    if not cfg then cfg = goop.config("blog_config", {
        layout = "list", blog_title = "My Blog",
        blog_subtitle = "Thoughts, stories & notes",
        accent = "#b44d2d", font = "serif", theme = "light",
    }) end
end

local function slugify(title)
    local s = string.lower(title)
    s = string.gsub(s, "[^a-z0-9]+", "-")
    s = string.gsub(s, "^%-+", "")
    s = string.gsub(s, "%-+$", "")
    return s
end

local function page()
    init()
    local is_owner = goop.peer.id == goop.self.id
    local is_coauthor = goop.group.is_member()
    return {
        posts = posts:find({ where = "published = 1", order = "_id DESC", limit = 50 }) or {},
        config = {
            layout = cfg.layout, blog_title = cfg.blog_title,
            blog_subtitle = cfg.blog_subtitle, accent = cfg.accent,
            font = cfg.font, theme = cfg.theme,
        },
        can_write = is_owner or is_coauthor,
        can_admin = is_owner,
    }
end

local function get_post(p)
    init()
    local slug = p.slug
    if not slug or slug == "" then return { found = false } end

    local row = posts:find_one({ where = "slug = ? AND published = 1", args = { slug } })
    if not row then
        row = posts:find_one({ where = "_id = ? AND published = 1", args = { slug } })
    end
    if not row then return { found = false } end
    return { found = true, post = row }
end

local function get_config()
    init()
    return {
        layout = cfg.layout, blog_title = cfg.blog_title,
        blog_subtitle = cfg.blog_subtitle, accent = cfg.accent,
        font = cfg.font, theme = cfg.theme,
    }
end

local function save_config(p)
    init()
    if not p.key or p.key == "" then error("key required") end
    cfg:set(p.key, p.value or "")
    return { ok = true }
end

local function save_post(p)
    init()
    local title = p.title or ""
    local body = p.body or ""
    if title == "" or body == "" then error("title and body required") end

    local slug = slugify(title)
    local id = tonumber(p.id)

    if id and id > 0 then
        posts:update(id, { title = title, body = body, slug = slug, image = p.image or "" })
        return { id = id }
    else
        local new_id = posts:insert({
            title = title, body = body, slug = slug,
            author_name = goop.peer.label or "", image = p.image or "", published = 1,
        })
        return { id = new_id }
    end
end

local function delete_post(p)
    init()
    local id = tonumber(p.id)
    if not id then error("id required") end
    local row = posts:get(id)
    if not row then error("post not found") end
    posts:delete(id)
    return { ok = true, image = row.image or "" }
end

local dispatch = goop.route({
    page = page,
    get_post = get_post,
    list_posts = function() init(); return { posts = posts:find({ where = "published = 1", order = "_id DESC", limit = 50 }) or {} } end,
    get_config = get_config,
    save_config = goop.owner(save_config),
    save_post = goop.coauthor(save_post),
    delete_post = goop.coauthor(delete_post),
})

function call(req) return dispatch(req) end

function handle(args)
    return "Visit my site to read my blog!"
end
