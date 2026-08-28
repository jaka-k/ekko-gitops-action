- Hits the 403: if retag-image ever runs from a different repo's workflow — say a promotion workflow living in the ekko GitOps repo that retags dev-abc1234 → prod-v1.2.3. The ekko repo's GITHUB_TOKEN isn't on       
  myapp's package ACL.

  The fix for that second case is a one-time grant per package: on the package page (github.com/users/jaka-k/packages/container/myapp) → Package settings → Manage Actions access → Add repository, add ekko with Write  
  role. After that its GITHUB_TOKEN pushes fine. The alternative is a PAT with write:packages, which acts as you and sidesteps the ACL — but then you're back to managing a long-lived token, which is what we were      
  avoiding.

  So the takeaway: as long as retagging stays in the same workflow that built the image (which is your current design — one composite action doing build → retag → update manifests), you never see this. It only becomes
  relevant if you later split promotion into a separate repo's workflow.