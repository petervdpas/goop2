--- Chess move validation and execution
--- @rate_limit 0

-- Piece values for AI evaluation
local PIECE_VALUES = { p = 100, n = 320, b = 330, r = 500, q = 900, k = 20000 }

function call(request)
    local p = request.params
    local game_id = p.game_id
    local from = p.from  -- e.g. "e2"
    local to = p.to      -- e.g. "e4"
    local promotion = p.promotion  -- e.g. "q" for queen

    if not game_id or not from or not to then
        error("game_id, from, and to required")
    end

    -- Load the game
    local rows = goop.db.query(
        "SELECT _id, _owner, challenger, board, status, mode, moves FROM games WHERE _id = ?",
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

    -- Parse FEN
    local fen = parse_fen(game.board)

    -- Determine caller's color
    local color
    if game.mode == "pve" then
        if goop.peer.id == game._owner then
            color = "w"
        else
            return { error = "you are not a player in this game" }
        end
    else
        if goop.peer.id == game._owner then
            color = "w"
        elseif goop.peer.id == game.challenger then
            color = "b"
        else
            return { error = "you are not a player in this game" }
        end
    end

    -- Check turn
    if fen.turn ~= color then
        return { error = "not your turn" }
    end

    -- Validate and make the move
    local result = make_move(fen, from, to, promotion)
    if result.error then
        return result
    end

    local new_fen = result.fen
    local move_notation = from .. to .. (promotion or "")
    local new_moves = game.moves
    if new_moves and new_moves ~= "" then
        new_moves = new_moves .. " " .. move_notation
    else
        new_moves = move_notation
    end

    -- Check game end conditions
    local new_status = "playing"
    local winner_id = ""
    local opponent_color = (color == "w") and "b" or "w"

    local parsed_new = parse_fen(new_fen)
    if is_checkmate(parsed_new, opponent_color) then
        new_status = "finished"
        if color == "w" then
            winner_id = game._owner
        else
            if game.mode == "pve" then
                winner_id = "__computer__"
            else
                winner_id = game.challenger
            end
        end
    elseif is_stalemate(parsed_new, opponent_color) then
        new_status = "draw"
    end

    -- If PvE and game still playing, computer moves
    if game.mode == "pve" and new_status == "playing" then
        local ai_move = pick_ai_move(parsed_new)
        if ai_move then
            local ai_result = make_move(parsed_new, ai_move.from, ai_move.to, ai_move.promotion)
            if not ai_result.error then
                new_fen = ai_result.fen
                local ai_notation = ai_move.from .. ai_move.to .. (ai_move.promotion or "")
                new_moves = new_moves .. " " .. ai_notation

                -- Re-check after AI move
                parsed_new = parse_fen(new_fen)
                if is_checkmate(parsed_new, "w") then
                    new_status = "finished"
                    winner_id = "__computer__"
                elseif is_stalemate(parsed_new, "w") then
                    new_status = "draw"
                end
            end
        end
    end

    -- Persist
    goop.db.exec(
        "UPDATE games SET board = ?, status = ?, winner = ?, moves = ?, _updated_at = CURRENT_TIMESTAMP WHERE _id = ?",
        new_fen, new_status, winner_id, new_moves, game_id
    )

    local final_fen = parse_fen(new_fen)
    return {
        game_id = game._id,
        fen = new_fen,
        turn = final_fen.turn,
        status = new_status,
        winner = winner_id,
        your_color = color,
        mode = game.mode,
        moves = new_moves,
        in_check = is_in_check(final_fen, final_fen.turn)
    }
end

-- Parse FEN string into components
function parse_fen(fen_str)
    local parts = {}
    for part in string.gmatch(fen_str, "[^%s]+") do
        table.insert(parts, part)
    end

    local board = {}
    local rank = 8
    local file = 1
    for i = 1, #parts[1] do
        local c = string.sub(parts[1], i, i)
        if c == "/" then
            rank = rank - 1
            file = 1
        elseif tonumber(c) then
            file = file + tonumber(c)
        else
            local sq = file_rank_to_sq(file, rank)
            board[sq] = c
            file = file + 1
        end
    end

    return {
        board = board,
        turn = parts[2] or "w",
        castling = parts[3] or "-",
        en_passant = parts[4] or "-",
        halfmove = tonumber(parts[5]) or 0,
        fullmove = tonumber(parts[6]) or 1
    }
end

-- Convert board state back to FEN
function to_fen(state)
    local fen = ""
    for rank = 8, 1, -1 do
        local empty = 0
        for file = 1, 8 do
            local sq = file_rank_to_sq(file, rank)
            local piece = state.board[sq]
            if piece then
                if empty > 0 then
                    fen = fen .. empty
                    empty = 0
                end
                fen = fen .. piece
            else
                empty = empty + 1
            end
        end
        if empty > 0 then
            fen = fen .. empty
        end
        if rank > 1 then
            fen = fen .. "/"
        end
    end

    fen = fen .. " " .. state.turn
    fen = fen .. " " .. (state.castling ~= "" and state.castling or "-")
    fen = fen .. " " .. state.en_passant
    fen = fen .. " " .. state.halfmove
    fen = fen .. " " .. state.fullmove

    return fen
end

-- Square helpers
function file_rank_to_sq(file, rank)
    return string.char(96 + file) .. rank
end

function sq_to_file_rank(sq)
    local file = string.byte(sq, 1) - 96
    local rank = tonumber(string.sub(sq, 2, 2))
    return file, rank
end

function is_valid_square(sq)
    if #sq ~= 2 then return false end
    local f, r = sq_to_file_rank(sq)
    return f >= 1 and f <= 8 and r >= 1 and r <= 8
end

-- Get piece color
function piece_color(piece)
    if not piece then return nil end
    if piece == string.upper(piece) then return "w" end
    return "b"
end

-- Make a move, returns new FEN or error
function make_move(state, from, to, promotion)
    if not is_valid_square(from) or not is_valid_square(to) then
        return { error = "invalid square" }
    end

    local piece = state.board[from]
    if not piece then
        return { error = "no piece at " .. from }
    end

    local color = piece_color(piece)
    if color ~= state.turn then
        return { error = "wrong color piece" }
    end

    -- Check if move is legal
    local legal_moves = get_legal_moves(state, from)
    local is_legal = false
    for _, m in ipairs(legal_moves) do
        if m == to then
            is_legal = true
            break
        end
    end

    if not is_legal then
        return { error = "illegal move" }
    end

    -- Create new state
    local new_board = {}
    for sq, p in pairs(state.board) do
        new_board[sq] = p
    end

    local new_castling = state.castling
    local new_en_passant = "-"
    local new_halfmove = state.halfmove + 1
    local new_fullmove = state.fullmove

    local piece_type = string.lower(piece)
    local from_file, from_rank = sq_to_file_rank(from)
    local to_file, to_rank = sq_to_file_rank(to)

    -- Handle special moves

    -- Pawn moves
    if piece_type == "p" then
        new_halfmove = 0

        -- Double pawn push - set en passant
        if math.abs(to_rank - from_rank) == 2 then
            local ep_rank = (from_rank + to_rank) / 2
            new_en_passant = file_rank_to_sq(from_file, ep_rank)
        end

        -- En passant capture
        if to == state.en_passant then
            local captured_rank = (color == "w") and (to_rank - 1) or (to_rank + 1)
            new_board[file_rank_to_sq(to_file, captured_rank)] = nil
        end

        -- Promotion
        if (color == "w" and to_rank == 8) or (color == "b" and to_rank == 1) then
            local promo_piece = promotion or "q"
            if color == "w" then
                piece = string.upper(promo_piece)
            else
                piece = string.lower(promo_piece)
            end
        end
    end

    -- Castling
    if piece_type == "k" then
        -- King moved - remove castling rights
        if color == "w" then
            new_castling = string.gsub(new_castling, "K", "")
            new_castling = string.gsub(new_castling, "Q", "")
        else
            new_castling = string.gsub(new_castling, "k", "")
            new_castling = string.gsub(new_castling, "q", "")
        end

        -- Kingside castling
        if to_file - from_file == 2 then
            local rook_from = file_rank_to_sq(8, from_rank)
            local rook_to = file_rank_to_sq(6, from_rank)
            new_board[rook_to] = new_board[rook_from]
            new_board[rook_from] = nil
        end
        -- Queenside castling
        if from_file - to_file == 2 then
            local rook_from = file_rank_to_sq(1, from_rank)
            local rook_to = file_rank_to_sq(4, from_rank)
            new_board[rook_to] = new_board[rook_from]
            new_board[rook_from] = nil
        end
    end

    -- Rook moves - remove castling rights
    if piece_type == "r" then
        if from == "a1" then new_castling = string.gsub(new_castling, "Q", "") end
        if from == "h1" then new_castling = string.gsub(new_castling, "K", "") end
        if from == "a8" then new_castling = string.gsub(new_castling, "q", "") end
        if from == "h8" then new_castling = string.gsub(new_castling, "k", "") end
    end

    -- Capture resets halfmove clock
    if state.board[to] then
        new_halfmove = 0
    end

    -- Make the move
    new_board[to] = piece
    new_board[from] = nil

    -- Update fullmove after black's turn
    if color == "b" then
        new_fullmove = new_fullmove + 1
    end

    -- Clean up empty castling
    if new_castling == "" then new_castling = "-" end

    local new_state = {
        board = new_board,
        turn = (color == "w") and "b" or "w",
        castling = new_castling,
        en_passant = new_en_passant,
        halfmove = new_halfmove,
        fullmove = new_fullmove
    }

    return { fen = to_fen(new_state) }
end

-- Get all legal moves for a piece
function get_legal_moves(state, from)
    local piece = state.board[from]
    if not piece then return {} end

    local color = piece_color(piece)
    local pseudo_moves = get_pseudo_moves(state, from)
    local legal = {}

    for _, to in ipairs(pseudo_moves) do
        -- Try the move and check if king is in check
        local new_board = {}
        for sq, p in pairs(state.board) do
            new_board[sq] = p
        end

        local piece_type = string.lower(piece)
        local from_file, from_rank = sq_to_file_rank(from)
        local to_file, to_rank = sq_to_file_rank(to)

        -- Handle en passant capture
        if piece_type == "p" and to == state.en_passant then
            local captured_rank = (color == "w") and (to_rank - 1) or (to_rank + 1)
            new_board[file_rank_to_sq(to_file, captured_rank)] = nil
        end

        -- Handle castling - check path is not attacked
        if piece_type == "k" and math.abs(to_file - from_file) == 2 then
            local step = (to_file > from_file) and 1 or -1
            local blocked = false
            for f = from_file, to_file, step do
                local sq = file_rank_to_sq(f, from_rank)
                if is_square_attacked(state, sq, (color == "w") and "b" or "w") then
                    blocked = true
                    break
                end
            end
            if blocked then
                goto continue
            end

            -- Move rook for castling check
            if to_file - from_file == 2 then
                local rook_from = file_rank_to_sq(8, from_rank)
                local rook_to = file_rank_to_sq(6, from_rank)
                new_board[rook_to] = new_board[rook_from]
                new_board[rook_from] = nil
            end
            if from_file - to_file == 2 then
                local rook_from = file_rank_to_sq(1, from_rank)
                local rook_to = file_rank_to_sq(4, from_rank)
                new_board[rook_to] = new_board[rook_from]
                new_board[rook_from] = nil
            end
        end

        new_board[to] = piece
        new_board[from] = nil

        local test_state = {
            board = new_board,
            turn = state.turn,
            castling = state.castling,
            en_passant = state.en_passant
        }

        if not is_in_check(test_state, color) then
            table.insert(legal, to)
        end

        ::continue::
    end

    return legal
end

-- Get pseudo-legal moves (doesn't check if king is in check after)
function get_pseudo_moves(state, from)
    local piece = state.board[from]
    if not piece then return {} end

    local color = piece_color(piece)
    local piece_type = string.lower(piece)
    local from_file, from_rank = sq_to_file_rank(from)
    local moves = {}

    if piece_type == "p" then
        local dir = (color == "w") and 1 or -1
        local start_rank = (color == "w") and 2 or 7

        -- Forward move
        local fwd = file_rank_to_sq(from_file, from_rank + dir)
        if is_valid_square(fwd) and not state.board[fwd] then
            table.insert(moves, fwd)
            -- Double push from start
            if from_rank == start_rank then
                local fwd2 = file_rank_to_sq(from_file, from_rank + 2 * dir)
                if not state.board[fwd2] then
                    table.insert(moves, fwd2)
                end
            end
        end

        -- Captures
        for _, df in ipairs({-1, 1}) do
            local cap_sq = file_rank_to_sq(from_file + df, from_rank + dir)
            if is_valid_square(cap_sq) then
                local target = state.board[cap_sq]
                if (target and piece_color(target) ~= color) or cap_sq == state.en_passant then
                    table.insert(moves, cap_sq)
                end
            end
        end

    elseif piece_type == "n" then
        local knight_moves = {
            {-2, -1}, {-2, 1}, {-1, -2}, {-1, 2},
            {1, -2}, {1, 2}, {2, -1}, {2, 1}
        }
        for _, d in ipairs(knight_moves) do
            local sq = file_rank_to_sq(from_file + d[1], from_rank + d[2])
            if is_valid_square(sq) then
                local target = state.board[sq]
                if not target or piece_color(target) ~= color then
                    table.insert(moves, sq)
                end
            end
        end

    elseif piece_type == "b" then
        add_sliding_moves(state, from, color, {{1,1}, {1,-1}, {-1,1}, {-1,-1}}, moves)

    elseif piece_type == "r" then
        add_sliding_moves(state, from, color, {{1,0}, {-1,0}, {0,1}, {0,-1}}, moves)

    elseif piece_type == "q" then
        add_sliding_moves(state, from, color, {
            {1,0}, {-1,0}, {0,1}, {0,-1},
            {1,1}, {1,-1}, {-1,1}, {-1,-1}
        }, moves)

    elseif piece_type == "k" then
        local king_moves = {
            {-1,-1}, {-1,0}, {-1,1}, {0,-1}, {0,1}, {1,-1}, {1,0}, {1,1}
        }
        for _, d in ipairs(king_moves) do
            local sq = file_rank_to_sq(from_file + d[1], from_rank + d[2])
            if is_valid_square(sq) then
                local target = state.board[sq]
                if not target or piece_color(target) ~= color then
                    table.insert(moves, sq)
                end
            end
        end

        -- Castling
        local back_rank = (color == "w") and 1 or 8
        if from_rank == back_rank and from_file == 5 then
            -- Kingside
            local can_ks = (color == "w" and string.find(state.castling, "K")) or
                          (color == "b" and string.find(state.castling, "k"))
            if can_ks then
                local f_sq = file_rank_to_sq(6, back_rank)
                local g_sq = file_rank_to_sq(7, back_rank)
                if not state.board[f_sq] and not state.board[g_sq] then
                    table.insert(moves, g_sq)
                end
            end
            -- Queenside
            local can_qs = (color == "w" and string.find(state.castling, "Q")) or
                          (color == "b" and string.find(state.castling, "q"))
            if can_qs then
                local b_sq = file_rank_to_sq(2, back_rank)
                local c_sq = file_rank_to_sq(3, back_rank)
                local d_sq = file_rank_to_sq(4, back_rank)
                if not state.board[b_sq] and not state.board[c_sq] and not state.board[d_sq] then
                    table.insert(moves, c_sq)
                end
            end
        end
    end

    return moves
end

function add_sliding_moves(state, from, color, directions, moves)
    local from_file, from_rank = sq_to_file_rank(from)

    for _, d in ipairs(directions) do
        local f, r = from_file + d[1], from_rank + d[2]
        while f >= 1 and f <= 8 and r >= 1 and r <= 8 do
            local sq = file_rank_to_sq(f, r)
            local target = state.board[sq]
            if target then
                if piece_color(target) ~= color then
                    table.insert(moves, sq)
                end
                break
            end
            table.insert(moves, sq)
            f = f + d[1]
            r = r + d[2]
        end
    end
end

-- Check if a square is attacked by a color
function is_square_attacked(state, sq, by_color)
    for from_sq, piece in pairs(state.board) do
        if piece_color(piece) == by_color then
            local attacks = get_attack_squares(state, from_sq)
            for _, a in ipairs(attacks) do
                if a == sq then return true end
            end
        end
    end
    return false
end

-- Get squares a piece attacks (similar to pseudo moves but for pawns only diagonal)
function get_attack_squares(state, from)
    local piece = state.board[from]
    if not piece then return {} end

    local piece_type = string.lower(piece)
    local color = piece_color(piece)
    local from_file, from_rank = sq_to_file_rank(from)

    if piece_type == "p" then
        local dir = (color == "w") and 1 or -1
        local attacks = {}
        for _, df in ipairs({-1, 1}) do
            local sq = file_rank_to_sq(from_file + df, from_rank + dir)
            if is_valid_square(sq) then
                table.insert(attacks, sq)
            end
        end
        return attacks
    end

    -- For other pieces, pseudo moves = attack squares
    return get_pseudo_moves(state, from)
end

-- Check if a color is in check
function is_in_check(state, color)
    -- Find king
    local king_sq = nil
    local king_char = (color == "w") and "K" or "k"
    for sq, piece in pairs(state.board) do
        if piece == king_char then
            king_sq = sq
            break
        end
    end

    if not king_sq then return false end

    local opp_color = (color == "w") and "b" or "w"
    return is_square_attacked(state, king_sq, opp_color)
end

-- Check for checkmate
function is_checkmate(state, color)
    if not is_in_check(state, color) then
        return false
    end
    return not has_legal_moves(state, color)
end

-- Check for stalemate
function is_stalemate(state, color)
    if is_in_check(state, color) then
        return false
    end
    return not has_legal_moves(state, color)
end

-- Check if color has any legal moves
function has_legal_moves(state, color)
    for sq, piece in pairs(state.board) do
        if piece_color(piece) == color then
            local moves = get_legal_moves(state, sq)
            if #moves > 0 then
                return true
            end
        end
    end
    return false
end

-- Simple AI: pick a reasonable move for black
function pick_ai_move(state)
    local all_moves = {}

    for sq, piece in pairs(state.board) do
        if piece_color(piece) == "b" then
            local moves = get_legal_moves(state, sq)
            for _, to in ipairs(moves) do
                table.insert(all_moves, { from = sq, to = to })
            end
        end
    end

    if #all_moves == 0 then return nil end

    -- Evaluate each move
    local best_move = nil
    local best_score = -999999

    for _, move in ipairs(all_moves) do
        local score = evaluate_move(state, move)
        if score > best_score then
            best_score = score
            best_move = move
        end
    end

    -- Add some randomness among top moves
    local top_moves = {}
    for _, move in ipairs(all_moves) do
        if evaluate_move(state, move) >= best_score - 50 then
            table.insert(top_moves, move)
        end
    end

    if #top_moves > 0 then
        return top_moves[math.random(#top_moves)]
    end

    return best_move
end

function evaluate_move(state, move)
    local score = 0

    -- Capture value
    local captured = state.board[move.to]
    if captured then
        local cap_type = string.lower(captured)
        score = score + (PIECE_VALUES[cap_type] or 0)
    end

    -- Avoid moving to attacked squares
    local opp_color = "w"
    if is_square_attacked(state, move.to, opp_color) then
        local moving_piece = string.lower(state.board[move.from])
        score = score - (PIECE_VALUES[moving_piece] or 0) / 2
    end

    -- Prefer center control
    local to_file, to_rank = sq_to_file_rank(move.to)
    local center_dist = math.abs(to_file - 4.5) + math.abs(to_rank - 4.5)
    score = score + (7 - center_dist) * 5

    -- Check if move gives check
    local new_board = {}
    for sq, p in pairs(state.board) do
        new_board[sq] = p
    end
    new_board[move.to] = new_board[move.from]
    new_board[move.from] = nil

    local test_state = {
        board = new_board,
        turn = "b",
        castling = state.castling,
        en_passant = state.en_passant
    }

    if is_in_check(test_state, "w") then
        score = score + 50
    end

    return score
end

function handle(args)
    return "Visit my site to play chess!"
end
