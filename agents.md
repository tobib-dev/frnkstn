## Writing code 

Prefer the smallest clear, correct change that fully solves the task.

Before editing, understand the affected flow and check whether existing 
code, the standard library, native platform features, or installed 
dependencies already solve it.

- Build only what the task requires. Avoid speculative abstractions, flexibilities, boilerplate, and unnecessary  dependencies.

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
