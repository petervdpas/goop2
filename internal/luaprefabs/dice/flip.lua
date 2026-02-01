-- flip.lua — Flip a coin.
function handle(args)
    math.randomseed(os.time())
    if math.random(1, 2) == 1 then
        return goop.peer.label .. " flipped: Heads!"
    else
        return goop.peer.label .. " flipped: Tails!"
    end
end
