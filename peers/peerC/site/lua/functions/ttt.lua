--- Tic-tac-toe game management
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
    elseif action == "cancel" then
        return cancel_game(request.params.game_id)
    elseif action == "stats" then
        return stats()
    else
        error("unknown action: " .. tostring(action))
    end
end

function new_game()
    -- Visitor challenges the host
    local existing = goop.db.query(
        "SELECT _id FROM games WHERE challenger = ? AND status IN ('waiting', 'playing') AND mode = 'pvp'",
        goop.peer.id
    )
    if existing and #existing > 0 then
        return { error = "you already have an active game", game_id = existing[1]._id }
    end

    goop.db.exec(
        "INSERT INTO games (_owner, challenger, challenger_label, mode, status) VALUES (?, ?, ?, 'pvp', 'waiting')",
        goop.self.id, goop.peer.id, goop.peer.label
    )

    local id = goop.db.scalar(
        "SELECT MAX(_id) FROM games WHERE challenger = ? AND mode = 'pvp'",
        goop.peer.id
    )

    return { game_id = id, your_symbol = "O", status = "waiting" }
end

function new_pve_game()
    -- Anyone can start a game against the computer
    goop.db.exec(
        "INSERT INTO games (_owner, challenger, challenger_label, mode, status, turn) VALUES (?, '__computer__', 'Computer', 'pve', 'playing', 'X')",
        goop.peer.id
    )

    local id = goop.db.scalar(
        "SELECT MAX(_id) FROM games WHERE _owner = ? AND mode = 'pve'",
        goop.peer.id
    )

    return {
        game_id = id,
        your_symbol = "X",
        status = "playing",
        board = "---------",
        turn = "X",
        mode = "pve"
    }
end

function game_state(game_id)
    if not game_id then
        error("game_id required")
    end

    local rows = goop.db.query(
        "SELECT _id, _owner, challenger, challenger_label, board, turn, status, winner, mode, _created_at, _updated_at FROM games WHERE _id = ?",
        game_id
    )
    if not rows or #rows == 0 then
        error("game not found")
    end
    local g = rows[1]

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

    -- Compute win line if game is over
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
        created_at = g._created_at
    }
    if win_line then
        result.win_line = win_line
    end
    return result
end

-- Reuse win-line detection from move.lua
local lines = {
    {1,2,3}, {4,5,6}, {7,8,9},
    {1,4,7}, {2,5,8}, {3,6,9},
    {1,5,9}, {3,5,7}
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
    local games = goop.db.query(
        "SELECT _id, _owner, challenger, challenger_label, board, turn, status, winner, mode, _created_at FROM games ORDER BY _id DESC LIMIT 20"
    )
    return { games = games or {} }
end

function cancel_game(game_id)
    if not game_id then
        error("game_id required")
    end

    local rows = goop.db.query(
        "SELECT _id, _owner, challenger, status FROM games WHERE _id = ?",
        game_id
    )
    if not rows or #rows == 0 then
        error("game not found")
    end
    local g = rows[1]

    if goop.peer.id ~= g._owner and goop.peer.id ~= g.challenger then
        return { error = "you are not a player in this game" }
    end
    if g.status ~= "waiting" and g.status ~= "playing" then
        return { error = "game is already finished" }
    end

    goop.db.exec(
        "UPDATE games SET status = 'cancelled', _updated_at = CURRENT_TIMESTAMP WHERE _id = ?",
        game_id
    )

    return { status = "cancelled" }
end

function stats()
    local wins = goop.db.scalar(
        "SELECT COUNT(*) FROM games WHERE winner = ? AND status LIKE 'won_%'",
        goop.self.id
    ) or 0
    local losses = goop.db.scalar(
        "SELECT COUNT(*) FROM games WHERE winner != '' AND winner != ? AND status LIKE 'won_%'",
        goop.self.id
    ) or 0
    local draws = goop.db.scalar(
        "SELECT COUNT(*) FROM games WHERE status = 'draw'"
    ) or 0

    return { wins = wins, losses = losses, draws = draws }
end

function handle(args)
    return "Visit my site to play tic-tac-toe!"
end
