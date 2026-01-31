---
description: Automatically synchronize project changes to GitHub
---

// turbo-all
1. Initialize git repository (if not already initialized)
```bash
git init
```

2. Add all changes to the staging area
```bash
git add .
```

3. Commit the changes with a descriptive message
```bash
git commit -m "Update: Project synchronization"
```

4. Add the remote origin (if needed)
```bash
# Note: User should provide their actual repository address if not already set
# git remote add origin <your-repo-address>
```

5. Push the changes to the main branch
```bash
git push -u origin main
```
