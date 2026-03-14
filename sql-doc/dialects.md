# SQL Dialect Guide (Optimized for AI)

This guide provides a comprehensive overview of the SQL dialects supported by Mayo by Teleskop.id. To ensure the AI generates accurate queries, we have separated the documentation into provider-specific guides.

## 🚀 Supported Providers

Select a provider below to view detailed syntax, date functions, and specific quirks:

- [**SQLite**](./dialects/sqlite.md) (Used for Dataframes, CSV, and XLSX imports)
- [**PostgreSQL**](./dialects/postgresql.md)
- [**MySQL**](./dialects/mysql.md)

---

## 🛠️ General Best Practices (Cross-Dialect)

Regardless of the database you are connected to, following these patterns will improve result accuracy and readability:

### 1. Read-Only Safety
Mayo by Teleskop.id is designed for **Analysis**. Only `SELECT` statements should be executed. Avoid `INSERT`, `UPDATE`, `DELETE`, or `DROP`.

### 2. Default Query Scope (Full Data)
By default, the AI will generate queries that select **ALL fields** (`SELECT *`) and **ALL rows** (No `LIMIT`), unless you explicitly ask for a sample or a specific number of rows. This ensures you see the complete picture by default.

### 3. Common Table Expressions (CTEs)
Use `WITH` clauses to break complex queries into logical steps. This makes it easier for the AI to debug and for you to understand the flow.
```sql
WITH monthly_sales AS (
    SELECT 
        DATE_TRUNC('month', order_date) as month,
        SUM(total_amount) as revenue
    FROM orders
    GROUP BY 1
)
SELECT * FROM monthly_sales WHERE revenue > 1000;
```

### 3. Explicit Aliasing
Always use the `AS` keyword for column and table aliases. It prevents ambiguity.
```sql
SELECT count(*) AS total_users FROM users;
```

### 4. Handling NULLs
Be proactive with `NULL` values to avoid "empty" looking reports.
- Use `COALESCE(column, 'N/A')` for strings.
- Use `COALESCE(column, 0)` for numeric aggregations.

### 5. Column Indexing in GROUP BY / ORDER BY
Most modern dialects allow using numbers to refer to columns in `GROUP BY` or `ORDER BY`.
```sql
SELECT category, region, SUM(sales)
FROM data
GROUP BY 1, 2
ORDER BY 3 DESC;
```

### 6. Case Sensitivity
- **Postgres**: Generally case-sensitive for string comparisons (use `ILIKE`).
- **MySQL/SQLite**: Often case-insensitive by default for basic strings, but behavior varies.
- **Tip**: When in doubt, use `LOWER(column) LIKE '%value%'`.

---

## 💡 Quick Syntax Comparison

| Feature | SQLite | PostgreSQL | MySQL |
|---------|--------|------------|-------|
| **Concat** | `a || b` | `a || b` or `CONCAT()` | `CONCAT(a, b)` |
| **Limit** | `LIMIT n OFFSET m` | `LIMIT n OFFSET m` | `LIMIT m, n` |
| **Current Date** | `date('now')` | `CURRENT_DATE` | `CURDATE()` |
| **JSON Extract** | `json_extract(c, '$.p')` | `c->>'p'` | `c->>'$.p'` |
| **Full Outer Join** | ❌ No | ✅ Yes | ❌ No |
