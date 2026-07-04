-- Seed data script for your Dev/Test environments
INSERT INTO orders (id, item, quantity, price) VALUES
('ord_01J1XYZ789', 'Wireless Ergonomic Keyboard', 1, 89.99),
('ord_02J2ABC456', 'USB-C Dual Monitor Docking Station', 1, 149.50)
ON CONFLICT (id) DO NOTHING;
