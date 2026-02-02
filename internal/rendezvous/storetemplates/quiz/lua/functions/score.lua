--- Score a quiz submission (server-side, answers stay hidden)
function call(request)
    local answers = request.params.answers
    if not answers then
        error("answers parameter required")
    end

    -- Load correct answers from the database
    local rows = goop.db.query("SELECT _id, correct FROM questions ORDER BY _id")
    if not rows or #rows == 0 then
        return { score = 0, total = 0, message = "No questions found." }
    end

    local score = 0
    local total = #rows
    for _, row in ipairs(rows) do
        local qid = tostring(row._id)
        if answers[qid] and string.lower(answers[qid]) == string.lower(row.correct) then
            score = score + 1
        end
    end

    -- Persist the score (one row per peer, update on resubmit)
    local existing = goop.db.scalar(
        "SELECT _id FROM scores WHERE _owner = ?", goop.peer.id
    )
    if existing then
        goop.db.exec(
            "UPDATE scores SET score = ?, total = ?, peer_label = ?, _updated_at = CURRENT_TIMESTAMP WHERE _owner = ?",
            score, total, goop.peer.label, goop.peer.id
        )
    else
        goop.db.exec(
            "INSERT INTO scores (_owner, score, total, peer_label) VALUES (?, ?, ?, ?)",
            goop.peer.id, score, total, goop.peer.label
        )
    end

    return {
        score = score,
        total = total,
        passed = score >= math.ceil(total * 0.7),
        message = score .. " out of " .. total .. " correct"
    }
end

function handle(args)
    return "This is a quiz. Visit my site to take it!"
end
