## Writing code 

Prefer the smallest clear, correct change that fully solves the task.

Before editing, understand the affected flow and check whether existing 
code, the standard library, native platform features, or installed 
dependencies already solve it.

- Build only what the task requires. Avoid speculative abstractions, flexibilities, boilerplate, and unnecessary  dependencies.
- Avoid magic values. Name any literal whose meaning is not obvious at the call site.
- For bug fixes, inspect every caller of the function being changed. Fix the shared root cause and check affected sibling paths.
- Prefer deletion and reuse. Never sacrifice correctness or readability to reduce line count or diff size.
- Suggest a simple approach when it meets the same requirements. Make routing implementation decisions without stopping for approval
- Comment only on non-obvious intent or constants. Mark deliberate shortcuts with a "ponytail" comment naming the limit and upgrade path.
- Keep handwritten source files at 1,000 lines or fewer. Split before exceeding the limit. Exclude generated files, lockfiles, and fixtures.

## Writing tests

-  Do not write excessive tests, only add a test if its failure would tell you something is actually broken.
- Assertions on styling values, colors, or internal structure fail on harmless changes and pass on real bugs, so leave them out.

## Writing style

- No em-dashes. Say what you mean. When a literal phrase is available, use it.

## Creating Database model queries

When creating database model queries do not write out SQL Strings such as the snippet below:

```
user.ID = gocql.TimeUUID()
	applied, err := db.Session.Session.Query(
		"INSERT INTO users_by_github_id (github_id, user_id, name, username) VALUES (?, ?, ?, ?) IF NOT EXISTS",
		user.GitHubID, user.ID, user.Name, user.Username,
	).WithContext(ctx).MapScanCAS(map[string]interface{}{})
```

Use the gocqlx query builder to construct queries instead, see sample below:

```
qb.Insert(usersByUsernameMetadata.Name).
			Columns("username", "id", "github_id", "name").
			Query(*db.Session)
		if err := batch.BindMap(usersByUsernameInsert, qb.M{
			"username":  user.Username,
			"id":        user.ID,
			"github_id": user.GitHubID,
			"name":      user.Name,
		}); err != nil {
			return User{}, err
		}
```
