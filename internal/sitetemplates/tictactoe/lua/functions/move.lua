--- Make a move in a tic-tac-toe game
function call(request)
    local p = request.params
    local game_id = p.game_id
    local pos = tonumber(p.position)

    if not game_id or not pos then
        error("game_id and position required")
    end
    if pos < 0 or pos > 8 then
        error("position must be 0-8")
    end

    -- Load the game
    local rows = goop.db.query(
        "SELECT _id, _owner, challenger, board, turn, status, mode FROM games WHERE _id = ?",
        game_id
    )
    if not rows or #rows == 0 then
        error("game not found")
    end
    local game = rows[1]

    -- Auto-start: host's first move begins a waiting PvP game
    if game.status == "waiting" then
        if goop.peer.id == game._owner then
            game.status = "playing"
        else
            return { error = "waiting for host to make the first move" }
        end
    end

    if game.status ~= "playing" then
        return { error = "game is not in progress", status = game.status }
    end

    -- Determine caller's symbol
    local symbol
    if game.mode == "pve" then
        if goop.peer.id == game._owner then
            symbol = "X"
        else
            return { error = "you are not a player in this game" }
        end
    else
        if goop.peer.id == game._owner then
            symbol = "X"
        elseif goop.peer.id == game.challenger then
            symbol = "O"
        else
            return { error = "you are not a player in this game" }
        end
    end

    -- Check turn
    if game.turn ~= symbol then
        return { error = "not your turn" }
    end

    -- Check cell is empty
    local idx = pos + 1
    local current = string.sub(game.board, idx, idx)
    if current ~= "-" then
        return { error = "cell is already occupied" }
    end

    -- Place the move
    local new_board = string.sub(game.board, 1, idx - 1)
                   .. symbol
                   .. string.sub(game.board, idx + 1)

    -- Check for winner
    local winner = check_winner(new_board)
    local new_status = "playing"
    local new_turn = (symbol == "X") and "O" or "X"
    local winner_id = ""
    local win_line = nil

    if winner then
        new_status = "won_" .. string.lower(winner)
        new_turn = ""
        win_line = get_win_line(new_board)
        if winner == "X" then
            winner_id = game._owner
        else
            if game.mode == "pve" then
                winner_id = "__computer__"
            else
                winner_id = game.challenger
            end
        end
    elseif not string.find(new_board, "-") then
        new_status = "draw"
        new_turn = ""
    end

    -- If PvE and game is still playing, computer moves immediately
    if game.mode == "pve" and new_status == "playing" then
        local ai_pos = pick_ai_move(new_board)
        if ai_pos then
            local ai_idx = ai_pos + 1
            new_board = string.sub(new_board, 1, ai_idx - 1)
                     .. "O"
                     .. string.sub(new_board, ai_idx + 1)

            -- Re-check after AI move
            winner = check_winner(new_board)
            if winner then
                new_status = "won_" .. string.lower(winner)
                new_turn = ""
                win_line = get_win_line(new_board)
                if winner == "X" then
                    winner_id = game._owner
                else
                    winner_id = "__computer__"
                end
            elseif not string.find(new_board, "-") then
                new_status = "draw"
                new_turn = ""
            else
                new_turn = "X"
            end
        end
    end

    -- Persist
    goop.db.exec(
        "UPDATE games SET board = ?, turn = ?, status = ?, winner = ?, _updated_at = CURRENT_TIMESTAMP WHERE _id = ?",
        new_board, new_turn, new_status, winner_id, game_id
    )

    local result = {
        board = new_board,
        turn = new_turn,
        status = new_status,
        winner = winner_id,
        your_symbol = symbol,
        mode = game.mode
    }
    if win_line then
        result.win_line = win_line
    end
    return result
end

-- All winning lines (1-indexed for Lua string.sub)
local lines = {
    {1,2,3}, {4,5,6}, {7,8,9},  -- rows
    {1,4,7}, {2,5,8}, {3,6,9},  -- cols
    {1,5,9}, {3,5,7}            -- diags
}

function check_winner(b)
    for _, line in ipairs(lines) do
        local a = string.sub(b, line[1], line[1])
        local b2 = string.sub(b, line[2], line[2])
        local c = string.sub(b, line[3], line[3])
        if a ~= "-" and a == b2 and b2 == c then
            return a
        end
    end
    return nil
end

function get_win_line(b)
    for _, line in ipairs(lines) do
        local a = string.sub(b, line[1], line[1])
        local b2 = string.sub(b, line[2], line[2])
        local c = string.sub(b, line[3], line[3])
        if a ~= "-" and a == b2 and b2 == c then
            -- Return 0-indexed positions
            return {line[1] - 1, line[2] - 1, line[3] - 1}
        end
    end
    return nil
end

-- AI: pick the best move for O
function pick_ai_move(board)
    -- 1. Win if possible
    local win = find_winning_move(board, "O")
    if win then return win end

    -- 2. Block opponent's win
    local block = find_winning_move(board, "X")
    if block then return block end

    -- 3. Take center
    if cell(board, 4) == "-" then return 4 end

    -- 4. Take a corner (prefer opposite of opponent's corner)
    local corners = {0, 2, 6, 8}
    local opposite = {[0]=8, [2]=6, [6]=2, [8]=0}
    for _, c in ipairs(corners) do
        if cell(board, c) == "X" and cell(board, opposite[c]) == "-" then
            return opposite[c]
        end
    end
    for _, c in ipairs(corners) do
        if cell(board, c) == "-" then return c end
    end

    -- 5. Take any edge
    local edges = {1, 3, 5, 7}
    for _, e in ipairs(edges) do
        if cell(board, e) == "-" then return e end
    end

    return nil
end

function cell(board, pos)
    return string.sub(board, pos + 1, pos + 1)
end

function find_winning_move(board, sym)
    for i = 0, 8 do
        if cell(board, i) == "-" then
            local test = string.sub(board, 1, i)
                      .. sym
                      .. string.sub(board, i + 2)
            if check_winner(test) == sym then
                return i
            end
        end
    end
    return nil
end

function handle(args)
    return "Visit my site to play tic-tac-toe!"
end
