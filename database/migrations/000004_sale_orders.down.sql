drop table if exists sale_order_items;
drop table if exists sale_order_entries;
drop trigger if exists trg_reject_system_sale_order_category_deletion on sale_order_categories;
drop table if exists sale_order_categories;
drop function if exists reject_system_sale_order_category_deletion();
