--- Make a move in a tic-tac-toe game — ORM queries via goop.schema
--- @rate_limit 0
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

    local game = goop.schema.get("games", game_id)
    if not game then
        error("game not found")
    end

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

    if game.turn ~= symbol then
        return { error = "not your turn" }
    end

    local idx = pos + 1
    local current = string.sub(game.board, idx, idx)
    if current ~= "-" then
        return { error = "cell is already occupied" }
    end

    local new_board = string.sub(game.board, 1, idx - 1)
                   .. symbol
                   .. string.sub(game.board, idx + 1)

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

    if game.mode == "pve" and new_status == "playing" then
        local ai_pos = pick_ai_move(new_board)
        if ai_pos then
            local ai_idx = ai_pos + 1
            new_board = string.sub(new_board, 1, ai_idx - 1)
                     .. "O"
                     .. string.sub(new_board, ai_idx + 1)

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

    goop.schema.update("games", game_id, {
        board = new_board,
        turn = new_turn,
        status = new_status,
        winner = winner_id,
    })

    local result = {
        game_id = game._id,
        board = new_board,
        turn = new_turn,
        status = new_status,
        winner = winner_id,
        your_symbol = symbol,
        mode = game.mode,
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
            return {line[1] - 1, line[2] - 1, line[3] - 1}
        end
    end
    return nil
end

function pick_ai_move(board)
    local win = find_winning_move(board, "O")
    if win then return win end

    local block = find_winning_move(board, "X")
    if block then return block end

    if cell(board, 4) == "-" then return 4 end

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
