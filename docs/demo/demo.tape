# VHS tape for the README demo GIF (https://github.com/charmbracelet/vhs).
# Regenerate from the repo root:
#   make build && cd docs/demo && vhs demo.tape
# The hidden block stages a scratch dir so the visible command is a clean
# `reap manifests/` against docs/demo/llm-finetune.yaml.
Output ../demo.gif

Set FontSize 15
Set Width 1150
Set Height 560
Set Padding 16
Set TypingSpeed 60ms

Hide
Type "export PATH=$PWD/../../bin:$PATH && mkdir -p /tmp/reap-demo/manifests && cp llm-finetune.yaml /tmp/reap-demo/manifests/ && cd /tmp/reap-demo && clear"
Enter
Show

Type "reap manifests/"
Sleep 500ms
Enter
Sleep 7s
