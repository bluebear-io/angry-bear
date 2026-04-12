# enforcement.tape -- Demonstrates enforcement (block then allow)
Output docs/assets/enforcement.gif
Set FontSize 14
Set Width 900
Set Height 500
Set Theme "Catppuccin Mocha"

Type "cat .care-bare/skill_enforcement.json"
Enter
Sleep 2s

Type@50ms "echo '{\"hook_event_name\":\"PreToolUse\",\"session_id\":\"demo\",\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"main.go\"}}' | care-bare hook --agent claude"
Enter
Sleep 2s

Type@50ms "echo '{\"hook_event_name\":\"PreToolUse\",\"session_id\":\"demo\",\"tool_name\":\"Skill\",\"tool_input\":{\"skill\":\"go-coding\"}}' | care-bare hook --agent claude"
Enter
Sleep 2s

Type@50ms "echo '{\"hook_event_name\":\"PreToolUse\",\"session_id\":\"demo\",\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"main.go\"}}' | care-bare hook --agent claude"
Enter
Sleep 2s

Type "care-bare status --session demo"
Enter
Sleep 3s
