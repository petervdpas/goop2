--- Chess game management
--- @rate_limit 0
function call(request)
    local action = request.params.action

    if action == "wait_for_game" then
        return wait_for_game()
    elseif action == "join_game" then
        return join_game(request.params.game_id)
    elseif action == "new_pve" then
        return new_pve_game()
    elseif action == "state" then
        return game_state(request.params.game_id)
    elseif action == "lobby" then
        return lobby()
    elseif action == "resign" then
        return resign_game(request.params.game_id)
    else
        error("unknown action: " .. tostring(action))
    end
end

function wait_for_game()
    -- Check if already in a game
    local existing = goop.db.query(
        "SELECT _id FROM games WHERE (_owner = ? OR challenger = ?) AND status IN ('waiting', 'playing') AND mode = 'pvp'",
        goop.peer.id, goop.peer.id
    )
    if existing and #existing > 0 then
        return { error = "you already have an active game", game_id = existing[1]._id }
    end

    -- Create a waiting game (owner is the one waiting, challenger is empty)
    goop.db.exec(
        "INSERT INTO games (_owner, _owner_label, challenger, challenger_label, mode, status) VALUES (?, ?, '', '', 'pvp', 'waiting')",
        goop.peer.id, goop.peer.label or "Anonymous"
    )

    local id = goop.db.scalar(
        "SELECT MAX(_id) FROM games WHERE _owner = ? AND mode = 'pvp'",
        goop.peer.id
    )

    return { game_id = id, your_color = "w", status = "waiting" }
end

function join_game(game_id)
    if not game_id then
        error("game_id required")
    end

    -- Get the waiting game
    local rows = goop.db.query(
        "SELECT _id, _owner, _owner_label, status FROM games WHERE _id = ? AND status = 'waiting' AND mode = 'pvp'",
        game_id
    )
    if not rows or #rows == 0 then
        return { error = "game not found or already started" }
    end
    local g = rows[1]

    -- Can't join your own game
    if g._owner == goop.peer.id then
        return { error = "cannot join your own game" }
    end

    -- Check if joining player already has an active game
    local existing = goop.db.query(
        "SELECT _id FROM games WHERE (_owner = ? OR challenger = ?) AND status IN ('waiting', 'playing') AND mode = 'pvp' AND _id != ?",
        goop.peer.id, goop.peer.id, game_id
    )
    if existing and #existing > 0 then
        return { error = "you already have an active game", game_id = existing[1]._id }
    end

    -- Join the game
    goop.db.exec(
        "UPDATE games SET challenger = ?, challenger_label = ?, status = 'playing', _updated_at = CURRENT_TIMESTAMP WHERE _id = ?",
        goop.peer.id, goop.peer.label or "Anonymous", game_id
    )

    return {
        game_id = game_id,
        your_color = "b",
        status = "playing",
        fen = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
        mode = "pvp",
        opponent_label = g._owner_label or "Anonymous"
    }
end

function new_pve_game()
    -- Anyone can start a game against the computer
    goop.db.exec(
        "INSERT INTO games (_owner, challenger, challenger_label, mode, status) VALUES (?, '__computer__', 'Computer', 'pve', 'playing')",
        goop.peer.id
    )

    local id = goop.db.scalar(
        "SELECT MAX(_id) FROM games WHERE _owner = ? AND mode = 'pve'",
        goop.peer.id
    )

    return {
        game_id = id,
        your_color = "w",
        status = "playing",
        fen = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
        mode = "pve",
        winner = "",
        challenger_label = "Computer"
    }
end

function game_state(game_id)
    if not game_id then
        error("game_id required")
    end

    local rows = goop.db.query(
        "SELECT _id, _owner, challenger, challenger_label, board, status, winner, mode, moves, _created_at, _updated_at FROM games WHERE _id = ?",
        game_id
    )
    if not rows or #rows == 0 then
        error("game not found")
    end
    local g = rows[1]

    local your_color = ""
    if g.mode == "pve" then
        if goop.peer.id == g._owner then
            your_color = "w"
        end
    else
        if goop.peer.id == g._owner then
            your_color = "w"
        elseif goop.peer.id == g.challenger then
            your_color = "b"
        end
    end

    -- Parse FEN to get current turn
    local turn = "w"
    local parts = split_fen(g.board)
    if parts and parts[2] then
        turn = parts[2]
    end

    return {
        game_id = g._id,
        fen = g.board,
        turn = turn,
        status = g.status,
        winner = g.winner,
        your_color = your_color,
        challenger = g.challenger,
        challenger_label = g.challenger_label,
        mode = g.mode,
        moves = g.moves,
        created_at = g._created_at
    }
end

function lobby()
    local games = goop.db.query(
        "SELECT _id, _owner, _owner_label, challenger, challenger_label, board, status, winner, mode, _created_at FROM games ORDER BY _id DESC LIMIT 20"
    )

    -- Get waiting games (players looking for opponents) - exclude self
    local waiting = goop.db.query(
        "SELECT _id, _owner, _owner_label, _created_at FROM games WHERE status = 'waiting' AND mode = 'pvp' AND _owner != ? ORDER BY _created_at ASC",
        goop.peer.id
    )

    -- Include stats for the site owner
    local wins = goop.db.scalar(
        "SELECT COUNT(*) FROM games WHERE winner = ? AND status = 'finished'",
        goop.self.id
    ) or 0
    local losses = goop.db.scalar(
        "SELECT COUNT(*) FROM games WHERE winner != '' AND winner != ? AND status = 'finished'",
        goop.self.id
    ) or 0
    local draws = goop.db.scalar(
        "SELECT COUNT(*) FROM games WHERE status = 'draw'"
    ) or 0

    return {
        games = games or {},
        waiting = waiting or {},
        stats = { wins = wins, losses = losses, draws = draws }
    }
end

function resign_game(game_id)
    if not game_id then
        error("game_id required")
    end

    local rows = goop.db.query(
        "SELECT _id, _owner, challenger, status, mode FROM games WHERE _id = ?",
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

    -- The other player wins
    local winner_id = ""
    if goop.peer.id == g._owner then
        winner_id = g.challenger
    else
        winner_id = g._owner
    end

    goop.db.exec(
        "UPDATE games SET status = 'finished', winner = ?, _updated_at = CURRENT_TIMESTAMP WHERE _id = ?",
        winner_id, game_id
    )

    return { status = "resigned", winner = winner_id }
end

-- Helper to split FEN string
function split_fen(fen)
    local parts = {}
    for part in string.gmatch(fen, "[^%s]+") do
        table.insert(parts, part)
    end
    return parts
end

function handle(args)
    return "Visit my site to play chess!"
end
