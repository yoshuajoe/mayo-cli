# MySQL Dialect Guide

MySQL (and MariaDB) is widely used and has some unique syntax for identifiers and limits.

## 1. Syntax Basics
- **Identifiers**: Use backticks `` `table_name` `` or `` `column name` `` for reserved words or names with spaces.
- **Case Sensitivity**: Table names can be case-sensitive depending on the OS (Linux is usually sensitive, macOS/Windows is not). Column names are usually case-insensitive.

## 2. Date and Time Handling
MySQL uses functions rather than operators for many date tasks.

| Task | Syntax | Example |
|------|--------|---------|
| Current Time | `NOW()` | `SELECT NOW();` |
| Format Date | `DATE_FORMAT(col, format)` | `DATE_FORMAT(dt, '%Y-%m-%d')` |
| Date Diff | `DATEDIFF(date1, date2)` | Returns days between. |
| Add Time | `DATE_ADD(col, INTERVAL 1 DAY)` | `DATE_ADD(now(), INTERVAL 1 MONTH)` |
| Extract | `YEAR(col)`, `MONTH(col)`, `DAY(col)`| Fast extraction. |

**Common Format codes for `DATE_FORMAT`**:
- `%Y`: Year (4 digits)
- `%m`: Month (01-12)
- `%d`: Day (01-31)
- `%H`: Hour (00-23)
- `%i`: Minutes (00-59)

## 3. String Manipulation
- **Concatenation**: Use `CONCAT(str1, str2, ...)`.
  - ⚠️ The `||` operator usually means `OR` in MySQL unless `PIPES_AS_CONCAT` mode is enabled.
  ```sql
  SELECT CONCAT(first_name, ' ', last_name) FROM users;
  ```
- **Length**: `LENGTH()` (bytes) or `CHAR_LENGTH()` (characters).
- **Substring**: `SUBSTRING(str, pos, len)`.

## 4. Limits and Paging
MySQL has a specific syntax for `LIMIT`:
- `LIMIT row_count`: Get first N rows.
- `LIMIT offset, row_count`: Skip `offset` rows and get `row_count` rows.
- `LIMIT row_count OFFSET offset`: Standard SQL syntax (also supported).

## 5. JSON Handling
Modern MySQL (5.7+) supports JSON.
- `JSON_EXTRACT(col, '$.path')`: Extract value.
- `col->'$.path'`: Shorthand for extract.
- `col->>'$.path'`: Extract value and **unquote** (get as text).

## 6. Mathematical Functions
- `ROUND(x, d)`, `CEIL(x)`, `FLOOR(x)`.
- `IFNULL(val, alt)`: MySQL equivalent of `COALESCE` for two arguments (though `COALESCE` is also supported).

## 7. Best Practices
- **GROUP BY**: MySQL is sometimes lenient with `GROUP BY` (non-aggregated columns), but it is safer to include all non-aggregated columns in the `GROUP BY` clause.
- **Joins**: Supports `INNER`, `LEFT`, `RIGHT`. ❌ `FULL OUTER JOIN` is **NOT** supported (use `UNION` of `LEFT` and `RIGHT` joins).
