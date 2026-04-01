--- Test: Game-like pattern (multi-table ORM, aggregates, state machine)
--- @rate_limit 0

local games = nil

function call(req)
    if not games then games = goop.orm("games") end
    local action = req.params.action

    if action == "new" then
        local id = games:insert({
            challenger = goop.peer.id,
            challenger_label = goop.peer.label,
            mode = req.params.mode or "pvp",
            status = "waiting",
            board = "---------",
            turn = "X",
            winner = "",
        })
        return { game_id = id, status = "waiting" }

    elseif action == "state" then
        local g = games:get(req.params.game_id)
        if not g then error("game not found") end
        return {
            game_id = g._id,
            status = g.status,
            board = g.board,
            turn = g.turn,
            winner = g.winner,
        }

    elseif action == "move" then
        local g = games:get(req.params.game_id)
        if not g then error("game not found") end
        if g.status ~= "waiting" and g.status ~= "playing" then
            return { error = "game is finished" }
        end
        games:update(req.params.game_id, {
            board = req.params.board,
            turn = req.params.turn,
            status = req.params.status or "playing",
            winner = req.params.winner or "",
        })
        return { ok = true }

    elseif action == "lobby" then
        local all = games:find({ order = "_id DESC", limit = 20 })
        local win_rows = games:aggregate("COUNT(*) as n", {
            where = "winner = ? AND status = 'finished'",
            args = { goop.self.id },
        })
        local wins = (win_rows and #win_rows > 0) and win_rows[1].n or 0
        return { games = all or {}, wins = wins }

    elseif action == "cancel" then
        games:update(req.params.game_id, { status = "cancelled" })
        return { ok = true }

    else
        error("unknown action: " .. tostring(action))
    end
end
