# Records the README demo. Needs vhs:  brew install vhs
#
#   python3 hack/demo/servers.py &   # two fake endpoints on 18080 and 18081
#   vhs hack/demo.tape               # writes .github/demo.gif
#
# The fake endpoints stand in for ingress-nginx and a Gateway: one redirects
# plain HTTP the way ssl-redirect does, the other forgot to.

Output .github/demo.gif

# Wide enough that the findings table does not wrap: a report that wraps
# reads as noise, which is the opposite of the point.
Set FontSize 13
Set Width 1800
Set Height 820
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms

Type "swapsafe scan --ingress-file hack/demo/manifests.yaml --httproute-file hack/demo/manifests.yaml"
Enter
Sleep 4s

Type "swapsafe run --baseline http://127.0.0.1:18080 --candidate http://127.0.0.1:18081 --ingress-file hack/demo/manifests.yaml --httproute-file hack/demo/manifests.yaml --fail-on major"
Enter
Sleep 7s

Type "echo $?"
Enter
Sleep 3s
