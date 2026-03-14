# PostgreSQL Dialect Guide

PostgreSQL is a powerful, feature-rich relational database. It is stricter than SQLite and MySQL regarding types.

## 1. Core Features
- **Full Join Support**: Supports `INNER`, `LEFT`, `RIGHT`, and `FULL OUTER` joins.
- **Type Safety**: Implicit type conversion is limited. Use explicit casting (`col::type`).

## 2. Date and Time Handling
PostgreSQL has rich date types and functions.

| Task | Syntax | Example |
|------|--------|---------|
| Current Timestamp | `NOW()` or `CURRENT_TIMESTAMP` | `SELECT NOW();` |
| Casting | `col::DATE` | `created_at::DATE` |
| Truncate Date | `DATE_TRUNC('unit', col)` | `DATE_TRUNC('month', order_date)` |
| Extract Part | `EXTRACT(field FROM col)` | `EXTRACT(YEAR FROM created_at)` |
| Intervals | `col + INTERVAL 'interval'` | `created_at + INTERVAL '1 day'` |
| Age | `AGE(timestamp)` | `AGE(birth_date)` |

**Common Units for `DATE_TRUNC`**: `'day'`, `'week'`, `'month'`, `'quarter'`, `'year'`.

## 3. String Manipulation
- **Concatenation**: `||` or `CONCAT(a, b, c)`.
- **Case Insensitivity**: Use `ILIKE` for case-insensitive matching.
  ```sql
  SELECT * FROM users WHERE email ILIKE '%@gmail.com';
  ```
- **Regex**: Use `~` (case-sensitive) or `~*` (case-insensitive).
- **Substrings**: `SUBSTRING(string FROM start FOR length)`.

## 4. Advanced Selection
- **DISTINCT ON**: Unique to Postgres, allows selecting the first row for each distinct value of a column.
  ```sql
  -- Get the latest order for each customer
  SELECT DISTINCT ON (customer_id) * 
  FROM orders 
  ORDER BY customer_id, order_date DESC;
  ```

## 5. JSON/JSONB Handling
Postgres is famous for its JSONB support.
- `col -> 'key'`: Returns JSON object field by key.
- `col ->> 'key'`: Returns JSON object field as **text**. (Most common for analysis).
- `col #> '{path,to,key}'`: Get JSON object at path.
- `col @> '{"key": "value"}'`: Check if JSON highlights a specific structure.

## 6. Window Functions
Full support for all standard window functions (`RANK`, `DENSE_RANK`, `ROW_NUMBER`, `LAG`, `LEAD`, etc.).

## 7. Best Practices
- **Quotes**: Use double quotes `"table_name"` only if the name has capitals or spaces. Otherwise, use snake_case.
- **Casting**: Always cast IDs or dates if performance seems slow or errors occur.
- **Limit**: Standard `LIMIT n OFFSET m` syntax.
