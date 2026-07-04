-- Bytebase will automatically track and execute this schema change
CREATE TABLE IF NOT EXISTS orders (
    id TEXT PRIMARY KEY,
    item TEXT NOT NULL,
    quantity INT NOT NULL,
    price NUMERIC(10, 2) NOT NULL
);
