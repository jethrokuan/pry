---
name: tug
description: Commit working copy changes and move the main bookmark to the new commit
disable-model-invocation: false
argument-hint: "[commit message]"
---

# Tug — Commit and advance main

Commit the current jj working copy and move the `main` bookmark forward to the new commit.

## Steps

1. Run `jj status` to verify there are uncommitted changes. If the working copy is empty, tell the user there's nothing to commit and stop.

2. Determine the commit message:
   - If `$ARGUMENTS` is provided, use it as the commit message.
   - Otherwise, run `jj diff --stat` and compose a short, descriptive commit message from the changes.

3. Commit:
   ```bash
   jj commit -m "<message>"
   ```

4. Move main to the new commit (`@-` is the just-created commit):
   ```bash
   jj bookmark set main -r @-
   ```

5. Confirm to the user with the commit hash and message.
