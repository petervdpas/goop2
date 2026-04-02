-- @rate_limit 0
function call(req)
  local cols = goop.orm("columns")
  local cfg = goop.orm("kanban_config")

  local n1 = cols:seed({
    {name = "To Do",       position = 0, color = "#6366f1"},
    {name = "In Progress", position = 1, color = "#f59e0b"},
    {name = "Done",        position = 2, color = "#22c55e"},
  })

  local n2 = cfg:seed({
    {key = "title",    value = "Kanban Board"},
    {key = "subtitle", value = "Shared team kanban board"},
  })

  return n1 + n2
end
