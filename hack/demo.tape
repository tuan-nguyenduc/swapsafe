# Records the README demo. Needs vhs:  brew install vhs
#
#   make build
#   python3 hack/demo/servers.py &       # two fake endpoints, no cluster
#   PATH="$PWD/bin:$PATH" vhs hack/demo.tape
#
# Three routes, three outcomes: one translated correctly, one with no
# HTTPRoute at all, one that lost its ssl-redirect.

Output .github/demo.gif

Set Shell bash
Set FontSize 22
# Wide enough that the findings table never wraps. A report that wraps reads
# as noise, which is the opposite of the point.
Set Width 1960
Set Height 900
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 28ms

# Every command below is real and copy-pasteable; m just keeps the lines short.
Hide
Type "clear" Enter
Type "m=hack/demo/manifests.yaml" Enter
Type "clear" Enter
Show

Type "# ingress2gateway translated three routes. Did the Gateway keep the behaviour?"
Enter
Sleep 1500ms

Type "swapsafe scan --ingress-file $m --httproute-file $m"
Enter
Sleep 4500ms

Type "# one route has no HTTPRoute at all. And the two that do?"
Enter
Sleep 1500ms

Type "swapsafe run --baseline http://127.0.0.1:18080 --candidate http://127.0.0.1:18081 --ingress-file $m --httproute-file $m"
Enter
Sleep 8s

Type "echo $?   # 2 means differences were found, so CI fails the PR"
Enter
Sleep 4s
