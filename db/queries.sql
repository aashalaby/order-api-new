-- name: GetOrders :many
SELECT id, item, quantity, price, user_id FROM orders
WHERE user_id = $1;

-- name: GetOrder :one
SELECT id, item, quantity, price, user_id FROM orders
WHERE id = $1 AND user_id = $2;

-- name: CreateOrder :one
INSERT INTO orders (id, item, quantity, price, user_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, item, quantity, price, user_id;

-- name: UpdateOrder :one
UPDATE orders
SET item = $2, quantity = $3, price = $4
WHERE id = $1 AND user_id = $5
RETURNING id, item, quantity, price, user_id;

-- name: DeleteOrder :execrows
DELETE FROM orders WHERE id = $1 AND user_id = $2;
