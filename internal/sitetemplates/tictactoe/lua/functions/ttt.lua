--- Tic-tac-toe game management — ORM queries via goop.schema
--- @rate_limit 0
function call(request)
    local action = request.params.action

    if action == "new" then
        return new_game()
    elseif action == "new_pve" then
        return new_pve_game()
    elseif action == "state" then
        return game_state(request.params.game_id)
    elseif action == "lobby" then
        return lobby()
    elseif action == "accept" then
        return accept_game(request.params.game_id)
    elseif action == "cancel" then
        return cancel_game(request.params.game_id)
    elseif action == "stats" then
        return stats()
    else
        error("unknown action: " .. tostring(action))
    end
end

function new_game()
    local existing = goop.schema.find_one("games", {
        where = "challenger = ? AND status IN ('waiting', 'playing') AND mode = 'pvp'",
        args = { goop.peer.id },
        fields = { "_id" },
    })
    if existing then
        return { error = "you already have an active game", game_id = existing._id }
    end

    local id = goop.schema.insert("games", {
        challenger = goop.peer.id,
        challenger_label = goop.peer.label,
        mode = "pvp",
        status = "waiting",
    })

    return { game_id = id, your_symbol = "O", status = "waiting" }
end

function new_pve_game()
    local id = goop.schema.insert("games", {
        challenger = "__computer__",
        challenger_label = "Computer",
        mode = "pve",
        status = "playing",
        turn = "X",
    })

    return {
        game_id = id,
        your_symbol = "X",
        status = "playing",
        board = "---------",
        turn = "X",
        mode = "pve",
        winner = "",
        challenger_label = "Computer",
    }
end

function game_state(game_id)
    if not game_id then
        error("game_id required")
    end

    local g = goop.schema.get("games", game_id)
    if not g then
        error("game not found")
    end

    local your_symbol = ""
    if g.mode == "pve" then
        if goop.peer.id == g._owner then
            your_symbol = "X"
        end
    else
        if goop.peer.id == g._owner then
            your_symbol = "X"
        elseif goop.peer.id == g.challenger then
            your_symbol = "O"
        end
    end

    local win_line = nil
    if g.status == "won_x" or g.status == "won_o" then
        win_line = get_win_line(g.board)
    end

    local result = {
        game_id = g._id,
        board = g.board,
        turn = g.turn,
        status = g.status,
        winner = g.winner,
        your_symbol = your_symbol,
        challenger_label = g.challenger_label,
        mode = g.mode,
        created_at = g._created_at,
    }
    if win_line then
        result.win_line = win_line
    end
    return result
end

local lines = {
    {1,2,3}, {4,5,6}, {7,8,9},
    {1,4,7}, {2,5,8}, {3,6,9},
    {1,5,9}, {3,5,7},
}

function get_win_line(b)
    for _, line in ipairs(lines) do
        local a = string.sub(b, line[1], line[1])
        local b2 = string.sub(b, line[2], line[2])
        local c = string.sub(b, line[3], line[3])
        if a ~= "-" and a == b2 and b2 == c then
            return {line[1] - 1, line[2] - 1, line[3] - 1}
        end
    end
    return nil
end

function lobby()
    local games = goop.schema.find("games", {
        order = "_id DESC",
        limit = 20,
    })

    local win_rows = goop.schema.aggregate("games", "COUNT(*) as n", {
        where = "winner = ? AND status LIKE 'won_%'",
        args = { goop.self.id },
    })
    local loss_rows = goop.schema.aggregate("games", "COUNT(*) as n", {
        where = "winner != '' AND winner != ? AND status LIKE 'won_%'",
        args = { goop.self.id },
    })
    local draw_rows = goop.schema.aggregate("games", "COUNT(*) as n", {
        where = "status = 'draw'",
    })

    return {
        games = games or {},
        stats = {
            wins = (win_rows and #win_rows > 0) and win_rows[1].n or 0,
            losses = (loss_rows and #loss_rows > 0) and loss_rows[1].n or 0,
            draws = (draw_rows and #draw_rows > 0) and draw_rows[1].n or 0,
        },
    }
end

function accept_game(game_id)
    if not game_id then
        error("game_id required")
    end

    local g = goop.schema.find_one("games", {
        where = "_id = ?",
        args = { game_id },
        fields = { "_id", "_owner", "status", "mode" },
    })
    if not g then
        error("game not found")
    end
    if g.status ~= "waiting" then
        return { error = "game is not waiting" }
    end
    if g.mode ~= "pvp" then
        return { error = "only pvp games can be accepted" }
    end
    if goop.peer.id ~= g._owner then
        return { error = "only the host can accept a challenge" }
    end

    goop.schema.update("games", game_id, { status = "playing" })

    return game_state(game_id)
end

function cancel_game(game_id)
    if not game_id then
        error("game_id required")
    end

    local g = goop.schema.find_one("games", {
        where = "_id = ?",
        args = { game_id },
        fields = { "_id", "_owner", "challenger", "status" },
    })
    if not g then
        error("game not found")
    end
    if goop.peer.id ~= g._owner and goop.peer.id ~= g.challenger then
        return { error = "you are not a player in this game" }
    end
    if g.status ~= "waiting" and g.status ~= "playing" then
        return { error = "game is already finished" }
    end

    goop.schema.update("games", game_id, { status = "cancelled" })

    return { status = "cancelled" }
end

function stats()
    local win_rows = goop.schema.aggregate("games", "COUNT(*) as n", {
        where = "winner = ? AND status LIKE 'won_%'",
        args = { goop.self.id },
    })
    local loss_rows = goop.schema.aggregate("games", "COUNT(*) as n", {
        where = "winner != '' AND winner != ? AND status LIKE 'won_%'",
        args = { goop.self.id },
    })
    local draw_rows = goop.schema.aggregate("games", "COUNT(*) as n", {
        where = "status = 'draw'",
    })

    return {
        wins = (win_rows and #win_rows > 0) and win_rows[1].n or 0,
        losses = (loss_rows and #loss_rows > 0) and loss_rows[1].n or 0,
        draws = (draw_rows and #draw_rows > 0) and draw_rows[1].n or 0,
    }
end

function handle(args)
    return "Visit my site to play tic-tac-toe!"
end
