--- Score a quiz submission (server-side, answers stay hidden)

local questions = nil
local scores = nil

function call(request)
    if not questions then questions = goop.orm("questions") end
    if not scores then scores = goop.orm("scores") end

    local answers = request.params.answers
    if not answers then
        error("answers parameter required")
    end

    local rows = questions:find({ fields = { "_id", "correct" }, order = "_id ASC" })
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

    local existing = scores:find_one({
        where = "_owner = ?",
        args = { goop.peer.id },
        fields = { "_id" },
    })
    if existing then
        scores:update(existing._id, {
            score = score,
            total = total,
            peer_label = goop.peer.label,
            email = goop.peer.email or "",
        })
    else
        scores:insert({
            score = score,
            total = total,
            peer_label = goop.peer.label,
            email = goop.peer.email or "",
        })
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
