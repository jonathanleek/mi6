# Example layers

Two folders. `home/.mi6/` is what `~/.mi6/` might hold. `tree/` is a git tree
like `~/Documents/git/` with a `.mi6/` at each level that needs one. The names
are made up. The shape mirrors a real one: a personal folder, a work folder
with a clients sub-folder and one client, a hobby folder, and `meta` for `mi6`
itself.

Copy the `.mi6/` folders you want into your own tree, then run `mi6 resolve`
in any repo under them to see which apply. The folders are hidden, so use
`ls -a` or `find . -name .mi6` to see them. [The design](../docs/design.md)
says what each file does.
