#!/usr/bin/env python3

import argparse
import sqlite3
import sys
from pathlib import Path


def quote_ident(value: str) -> str:
    return '"' + value.replace('"', '""') + '"'


def format_value(column: str, value, omit_message_body: bool) -> str:
    if value is None:
        return "NULL"
    if omit_message_body and column == "raw_message_body":
        if isinstance(value, bytes):
            return f"<omitted {len(value)} bytes>"
        return f"<omitted {len(str(value))} chars>"
    if isinstance(value, bytes):
        return "0x" + value.hex()
    return repr(value)


def print_table(conn: sqlite3.Connection, table_name: str, omit_message_body: bool) -> None:
    quoted_name = quote_ident(table_name)
    columns = [row[1] for row in conn.execute(f"PRAGMA table_info({quoted_name})")]

    print(f"\n## {table_name}")
    print(f"columns: {', '.join(columns) if columns else '(none)'}")

    rows = conn.execute(f"SELECT * FROM {quoted_name}").fetchall()
    if not rows:
        print("(no rows)")
        return

    for index, row in enumerate(rows, start=1):
        print(f"\nrow {index}:")
        for column, value in zip(columns, row):
            print(f"  {column}: {format_value(column, value, omit_message_body)}")


def main() -> int:
    parser = argparse.ArgumentParser(description="Print the schema and contents of a SQLite database.")
    parser.add_argument(
        "--omit-message-body",
        action="store_true",
        help="omit full raw_message_body values from row output",
    )
    parser.add_argument("location", help="Path to the SQLite database file")
    args = parser.parse_args()

    db_path = Path(args.location)
    if not db_path.exists():
        print(f"error: database does not exist: {db_path}", file=sys.stderr)
        return 1
    if not db_path.is_file():
        print(f"error: database path is not a file: {db_path}", file=sys.stderr)
        return 1

    conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    try:
        conn.row_factory = sqlite3.Row

        print(f"# {db_path}")
        print("\n## schema")
        schema_rows = conn.execute(
            """
            SELECT type, name, sql
            FROM sqlite_master
            WHERE name NOT LIKE 'sqlite_%'
            ORDER BY type, name
            """
        ).fetchall()

        if not schema_rows:
            print("(empty schema)")
            return 0

        for row in schema_rows:
            print(f"\n-- {row['type']}: {row['name']}")
            print(row["sql"] + ";")

        table_names = [
            row["name"]
            for row in schema_rows
            if row["type"] == "table"
        ]

        print("\n## tables")
        for table_name in table_names:
            print_table(conn, table_name, args.omit_message_body)
    finally:
        conn.close()

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
