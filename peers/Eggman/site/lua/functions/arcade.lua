--- Arcade game operations
--- @rate_limit 0
function call(request)
    local action = request.params.action

    if action == "submit_score" then
        return submit_score(request.params)
    elseif action == "get_leaderboard" then
        return get_leaderboard(request.params)
    elseif action == "get_level" then
        return get_level(request.params)
    elseif action == "save_level" then
        return save_level(request.params)
    else
        error("unknown action: " .. tostring(action))
    end
end

function submit_score(params)
    local score = params.score
    local level = params.level or 1
    local time_ms = params.time_ms or 0

    if not score or score < 0 then
        return { error = "invalid score" }
    end

    goop.db.exec(
        "INSERT INTO high_scores (_owner, player_label, score, level, time_ms) VALUES (?, ?, ?, ?, ?)",
        goop.peer.id,
        goop.peer.label or "Anonymous",
        score,
        level,
        time_ms
    )

    -- Get rank
    local rank = goop.db.scalar(
        "SELECT COUNT(*) + 1 FROM high_scores WHERE score > ?",
        score
    ) or 1

    return { status = "ok", rank = rank }
end

function get_leaderboard(params)
    local limit = params.limit or 10
    local scores = goop.db.query(
        "SELECT player_label, score, level, time_ms, _created_at FROM high_scores ORDER BY score DESC LIMIT ?",
        limit
    )
    return { scores = scores or {} }
end

function get_level(params)
    local level_num = params.level_num or 1
    local levels = goop.db.query(
        "SELECT _id, level_num, title, data FROM levels WHERE level_num = ? LIMIT 1",
        level_num
    )
    if not levels or #levels == 0 then
        return { error = "level not found" }
    end
    return levels[1]
end

function save_level(params)
    -- Owner only
    if goop.peer.id ~= goop.self.id then
        return { error = "only owner can save levels" }
    end

    local level_num = params.level_num
    local title = params.title or ""
    local data = params.data

    if not level_num or not data then
        return { error = "level_num and data required" }
    end

    -- Upsert
    local existing = goop.db.scalar(
        "SELECT _id FROM levels WHERE level_num = ?",
        level_num
    )

    if existing then
        goop.db.exec(
            "UPDATE levels SET title = ?, data = ? WHERE _id = ?",
            title, data, existing
        )
    else
        goop.db.exec(
            "INSERT INTO levels (_owner, level_num, title, data) VALUES (?, ?, ?, ?)",
            goop.self.id, level_num, title, data
        )
    end

    return { status = "saved" }
end

function handle(args)
    return "Play my arcade game!"
end
