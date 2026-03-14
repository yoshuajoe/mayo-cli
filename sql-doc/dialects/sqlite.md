# SQLite Dialect Guide

SQLite is the default engine for local dataframes and file imports (CSV, XLSX) in Mayo by Teleskop.id. It is lightweight but has some specific limitations and syntax nuances.

## 1. Important Limitations
- **Joins**: ONLY supports `INNER JOIN` and `LEFT JOIN`. 
  - ❌ `RIGHT JOIN` and `FULL OUTER JOIN` are **NOT** supported.
  - *Workaround for Full Outer Join*: Use a `LEFT JOIN` combined with `UNION` and another `LEFT JOIN` (swapping tables).
- **Data Types**: SQLite uses dynamic typing. Any type of data can be stored in any column. However, it's safest to treat columns as their intended types (`TEXT`, `INTEGER`, `REAL`, `BLOB`).

## 2. Date and Time Handling
SQLite does not have a native `DATE` or `DATETIME` storage class. Dates are stored as strings (ISO 8601), numbers, or reals.

| Task | Syntax | Example |
|------|--------|---------|
| Current Date | `date('now')` | `SELECT date('now');` |
| Current Time | `datetime('now')` | `SELECT datetime('now');` |
| Format Date | `strftime(format, col)` | `strftime('%Y-%m', created_at)` |
| Add Time | `date(col, '+1 day')` | `date('2023-01-01', '+7 days')` |
| Unix Epoch | `strftime('%s', 'now')` | Returns seconds since 1970 |

**Common Formats for `strftime`**:
- `%Y`: Year (2023)
- `%m`: Month (01-12)
- `%d`: Day (01-31)
- `%H`: Hour (00-24)
- `%M`: Minute (00-59)
- `%S`: Second (00-59)

## 3. String Manipulation
- **Concatenation**: Use the `||` operator.
  ```sql
  SELECT first_name || ' ' || last_name AS full_name FROM users;
  ```
- **Case Sensitivity**: By default, `LIKE` is case-insensitive for ASCII characters only.
- **Substring**: `SUBSTR(string, start, length)` (1-indexed).
- **Length**: `LENGTH(string)`.
- **Replace**: `REPLACE(string, pattern, replacement)`.

## 4. Mathematical Functions
- **Rounding**: `ROUND(value, decimals)`.
- **Absolute Value**: `ABS(value)`.
- **Truncation**: SQLite doesn't have `TRUNC()`. Use `CAST(x AS INT)`.

## 5. Window Functions
Modern SQLite (used here) supports standard window functions:
```sql
SELECT 
  name, 
  salary,
  RANK() OVER (PARTITION BY department ORDER BY salary DESC) as rank
FROM employees;
```

## 6. JSON Handling
Use these functions for JSON columns or strings:
- `json_extract(column, '$.path')`: Extract a value.
- `json_each(column)`: Table-valued function to iterate through arrays.

## 7. Best Practices
- **CTEs**: Use `WITH` clauses to break down complex logic.
- **Filtering**: Always filter dates using the ISO format `YYYY-MM-DD`.
- **NULLs**: Use `IFNULL(col, alternative)` or `COALESCE(col, alt1, alt2)` to handle missing data.
