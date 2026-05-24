# init.tape -- Demonstrates angry-bear init
Output docs/assets/init.gif
Set FontSize 14
Set Width 900
Set Height 500
Set Theme "Catppuccin Mocha"

Type "ls -la"
Enter
Sleep 1s

Type "mkdir .claude"
Enter
Sleep 500ms

Type "angry-bear init"
Enter
Sleep 3s

Type "cat .angry-bear/skill_enforcement.json"
Enter
Sleep 2s

Type "cat .claude/settings.json"
Enter
Sleep 3s
