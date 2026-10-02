# Issue tracker: GitHub Issues

The tracker of this project is **GitHub Issues**:
<https://github.com/nicolasalberti00/homey/issues>. The tracker lives online:
nothing that tracks state — plans, boards, session logs — is kept in this
repository.

## Conventions

- One feature, bug or decision per issue. The pull request that delivers it
  closes it (`Closes #NN`) and is reviewed before anything merges into `main`.
- Titles and bodies in English, like the rest of the repository, regardless of
  the language the session is held in.
- Triage uses the five roles in `triage-labels.md`; the GitHub label strings
  equal the role names (`needs-triage`, `needs-info`, `ready-for-agent`,
  `ready-for-human`, `wontfix`).
- Specs and tickets a skill publishes become issues on GitHub: one issue per
  ticket, dependencies expressed with the tracker's own links (a `Blocked by
  #NN` line in the body), never a single combined file.

## When a skill says "publish to the issue tracker"

Create the issues with the GitHub CLI:

```bash
gh issue create --title "…" --body "…"
```

A spec goes on the issue that tracks the feature (or gets its own issue when
the skill asks for one); each ticket becomes its own issue, cross-referencing
the others for its blocking edges.

## When a skill says "fetch the relevant ticket"

```bash
gh issue view <number>
```

The user will normally pass the issue number or the URL directly; otherwise
search by title with `gh issue list --search "…"`.
