---
name: "Bug Report"
about: "Use this template when reporting a bug, so you don't forget important information we'd ask for later."
title: "Bug: "
labels: 
- kind/bug
---

### describe your environment
- bh version used (`bh -v`):
    - [ ] I also reproduced the issue [with the latest main build](https://github.com/aayushpokhrel49/branchspire-cli/releases/download/main/)
- BranchSpire version used:
    - [ ] the issue only occurred after updating BranchSpire recently
- operating system:
- I make use of...
    - [ ] non-standard default branch names (no `main`,`master`, or `trunk`)
    - [ ] .ssh/config or .gitconfig host aliases in my git remotes
    - [ ] ssh_agent or similar
    - [ ] non-standard ports for BranchSpire and/or ssh
    - [ ] something else that's likely to interact badly with bh: ...


Please provide the output of `git remote -v` (if the issue is related to bh not finding resources on BranchSpire):
```

```

### describe the issue (observed vs expected behaviour)


