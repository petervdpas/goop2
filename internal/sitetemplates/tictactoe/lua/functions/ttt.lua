--- Tic-tac-toe game management
--- @rate_limit 0

local games = nil
local function init() if not games then games = goop.orm("games") end end

local lines = {
    {1,2,3}, {4,5,6}, {7,8,9},
    {1,4,7}, {2,5,8}, {3,6,9},
    {1,5,9}, {3,5,7},
}

local function get_win_line(b)
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

local function game_state(p)
    init()
    local game_id = p.game_id
    if not game_id then error("game_id required") end

    local g = games:get(game_id)
    if not g then error("game not found") end

    local your_symbol = ""
    if g.mode == "pve" then
        if goop.peer.id == g._owner then your_symbol = "X" end
    else
        if goop.peer.id == g._owner then your_symbol = "X"
        elseif goop.peer.id == g.challenger then your_symbol = "O" end
    end

    local result = {
        game_id = g._id, board = g.board, turn = g.turn,
        status = g.status, winner = g.winner, your_symbol = your_symbol,
        challenger_label = g.challenger_label, mode = g.mode, created_at = g._created_at,
    }
    if g.status == "won_x" or g.status == "won_o" then
        result.win_line = get_win_line(g.board)
    end
    return result
end

local function get_stats()
    local win_rows = games:aggregate("COUNT(*) as n", { where = "winner = ? AND status LIKE 'won_%'", args = { goop.self.id } })
    local loss_rows = games:aggregate("COUNT(*) as n", { where = "winner != '' AND winner != ? AND status LIKE 'won_%'", args = { goop.self.id } })
    local draw_rows = games:aggregate("COUNT(*) as n", { where = "status = 'draw'" })
    return {
        wins = (win_rows and #win_rows > 0) and win_rows[1].n or 0,
        losses = (loss_rows and #loss_rows > 0) and loss_rows[1].n or 0,
        draws = (draw_rows and #draw_rows > 0) and draw_rows[1].n or 0,
    }
end

local dispatch = goop.route({
    new = function()
        init()
        local existing = games:find_one({
            where = "challenger = ? AND status IN ('waiting', 'playing') AND mode = 'pvp'",
            args = { goop.peer.id }, fields = { "_id" },
        })
        if existing then
            return { error = "you already have an active game", game_id = existing._id }
        end
        local id = games:insert({
            challenger = goop.peer.id, challenger_label = goop.peer.label,
            mode = "pvp", status = "waiting",
        })
        return { game_id = id, your_symbol = "O", status = "waiting" }
    end,

    new_pve = function()
        init()
        local id = games:insert({
            challenger = "__computer__", challenger_label = "Computer",
            mode = "pve", status = "playing", turn = "X",
        })
        return {
            game_id = id, your_symbol = "X", status = "playing",
            board = "---------", turn = "X", mode = "pve",
            winner = "", challenger_label = "Computer",
        }
    end,

    state = game_state,

    lobby = function()
        init()
        return {
            games = games:find({ order = "_id DESC", limit = 20 }) or {},
            stats = get_stats(),
        }
    end,

    accept = function(p)
        init()
        local game_id = p.game_id
        if not game_id then error("game_id required") end
        local g = games:find_one({ where = "_id = ?", args = { game_id }, fields = { "_id", "_owner", "status", "mode" } })
        if not g then error("game not found") end
        if g.status ~= "waiting" then return { error = "game is not waiting" } end
        if g.mode ~= "pvp" then return { error = "only pvp games can be accepted" } end
        if goop.peer.id ~= g._owner then return { error = "only the host can accept a challenge" } end
        games:update(game_id, { status = "playing" })
        return game_state(p)
    end,

    cancel = function(p)
        init()
        local game_id = p.game_id
        if not game_id then error("game_id required") end
        local g = games:find_one({ where = "_id = ?", args = { game_id }, fields = { "_id", "_owner", "challenger", "status" } })
        if not g then error("game not found") end
        if goop.peer.id ~= g._owner and goop.peer.id ~= g.challenger then
            return { error = "you are not a player in this game" }
        end
        if g.status ~= "waiting" and g.status ~= "playing" then
            return { error = "game is already finished" }
        end
        games:update(game_id, { status = "cancelled" })
        return { status = "cancelled" }
    end,

    stats = function() init(); return get_stats() end,
})

function call(req) return dispatch(req) end

function handle(args)
    return "Visit my site to play tic-tac-toe!"
end
