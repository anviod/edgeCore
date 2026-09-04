import sqlite3
import json

conn = sqlite3.connect('/usr/local/bin/edgeCore/data/config.db')
cursor = conn.cursor()

# List tables
cursor.execute("SELECT name FROM sqlite_master WHERE type='table'")
tables = cursor.fetchall()
print('Tables:', tables)

# Check Users
cursor.execute("SELECT * FROM Users")
rows = cursor.fetchall()
for row in rows:
    print(f'User: {row}')

# Check Northbound config
cursor.execute("SELECT * FROM Northbound")
rows = cursor.fetchall()
for row in rows:
    print(f'Northbound: {row}')

# Check config bucket for bacnet_server
try:
    cursor.execute("SELECT key, value FROM ConfigVersion")
    rows = cursor.fetchall()
    for row in rows:
        print(f'ConfigVersion: {row[0]} = {row[1][:200]}')
except:
    print('ConfigVersion table not found')

conn.close()