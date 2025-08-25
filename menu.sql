-- ===========================
-- ENUMS
-- ===========================
CREATE TYPE user_type         AS ENUM ('guest', 'registered', 'admin');
CREATE TYPE auth_provider     AS ENUM ('password', 'google', 'otp');
CREATE TYPE payment_method    AS ENUM ('cash', 'stripe');
CREATE TYPE payment_status    AS ENUM ('pending', 'succeeded', 'failed', 'refunded', 'cancelled');
CREATE TYPE order_status      AS ENUM ('draft', 'pending', 'confirmed', 'preparing', 'ready', 'completed', 'cancelled', 'refunded');
CREATE TYPE subscription_plan AS ENUM ('free', 'premium');
CREATE TYPE subscription_status AS ENUM ('active', 'cancelled', 'trial');
CREATE TYPE rule_scope        AS ENUM ('venue', 'category', 'item');
CREATE TYPE rule_value_type   AS ENUM ('percent', 'fixed');
CREATE TYPE loyalty_tx_type   AS ENUM ('earn', 'redeem', 'adjust');
CREATE TYPE page_status       AS ENUM ('draft', 'published', 'archived');
CREATE TYPE builder_block_type AS ENUM (
  'section','category_list','item_card','item_grid','hero','banner','html','spacer','divider','button','image','carousel'
);
CREATE TYPE channel_type      AS ENUM ('qrscan', 'public_link', 'admin_pos');

-- ===========================
-- CORE: Users, Venues, Access
-- ===========================
CREATE TABLE users (
  id            BIGSERIAL PRIMARY KEY,
  type          user_type NOT NULL DEFAULT 'registered',
  name          TEXT,
  phone         TEXT UNIQUE,
  email         CITEXT UNIQUE,
  password_hash TEXT,
  status        TEXT DEFAULT 'active',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE user_auth_providers (
  id            BIGSERIAL PRIMARY KEY,
  user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider      auth_provider NOT NULL,
  provider_uid  TEXT,
  meta_json     JSONB DEFAULT '{}'::jsonb,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(user_id, provider)
);

CREATE TABLE venues (
  id            BIGSERIAL PRIMARY KEY,
  owner_user_id BIGINT NOT NULL REFERENCES users(id),
  name          TEXT NOT NULL,
  slug          CITEXT UNIQUE NOT NULL,            -- used for public link: /m/:slug
  is_public     BOOLEAN NOT NULL DEFAULT TRUE,     -- public menu link switch
  locale_default TEXT NOT NULL DEFAULT 'en',
  currency      TEXT NOT NULL DEFAULT 'IQD',
  address       TEXT,
  phone         TEXT,
  logo_url      TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX venues_slug_idx ON venues(slug);

CREATE TABLE venue_users (
  venue_id      BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  user_id       BIGINT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
  role          TEXT   NOT NULL DEFAULT 'owner',       -- owner/manager/staff
  PRIMARY KEY (venue_id, user_id),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ===========================
-- PUBLIC LINKS & QR CODES
-- ===========================
CREATE TABLE qr_codes (
  id          BIGSERIAL PRIMARY KEY,
  venue_id    BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  url         TEXT NOT NULL,                         -- resolved full URL
  table_no    TEXT,                                  -- optional: bind to table
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ===========================
-- MENU DATA (items, categories, add-ons, variants)
-- ===========================
CREATE TABLE menu_categories (
  id          BIGSERIAL PRIMARY KEY,
  venue_id    BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  sort_index  INT  NOT NULL DEFAULT 0,
  is_visible  BOOLEAN NOT NULL DEFAULT TRUE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX menu_categories_venue_sort_idx ON menu_categories(venue_id, sort_index);

CREATE TABLE menu_items (
  id            BIGSERIAL PRIMARY KEY,
  venue_id      BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  category_id   BIGINT REFERENCES menu_categories(id) ON DELETE SET NULL,
  name          TEXT NOT NULL,
  description   TEXT,
  price_base    NUMERIC(12,2) NOT NULL DEFAULT 0,
  image_url     TEXT,
  allergens     TEXT[],                              -- quick filter; details via i18n_strings if needed
  is_visible    BOOLEAN NOT NULL DEFAULT TRUE,
  sort_index    INT NOT NULL DEFAULT 0,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX menu_items_venue_category_idx ON menu_items(venue_id, category_id, sort_index);

CREATE TABLE item_variants (
  id          BIGSERIAL PRIMARY KEY,
  item_id     BIGINT NOT NULL REFERENCES menu_items(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,                          -- e.g., Small, Medium
  price_delta NUMERIC(12,2) NOT NULL DEFAULT 0
);

CREATE TABLE item_addons (
  id          BIGSERIAL PRIMARY KEY,
  item_id     BIGINT NOT NULL REFERENCES menu_items(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,                          -- e.g., Cheese, Fries
  price_delta NUMERIC(12,2) NOT NULL DEFAULT 0,
  is_default  BOOLEAN NOT NULL DEFAULT FALSE
);

-- ===========================
-- DRAG-AND-DROP MENU PAGE BUILDER
-- ===========================
-- A venue can build a complete public-facing menu page composed of blocks.

CREATE TABLE menu_pages (
  id          BIGSERIAL PRIMARY KEY,
  venue_id    BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  title       TEXT NOT NULL DEFAULT 'Menu',
  status      page_status NOT NULL DEFAULT 'published',
  version     INT NOT NULL DEFAULT 1,
  published_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE menu_page_blocks (
  id          BIGSERIAL PRIMARY KEY,
  page_id     BIGINT NOT NULL REFERENCES menu_pages(id) ON DELETE CASCADE,
  block_type  builder_block_type NOT NULL,
  sort_index  INT NOT NULL DEFAULT 0,
  config_json JSONB NOT NULL DEFAULT '{}'::jsonb,   -- layout/config (columns, style, etc.)
  data_json   JSONB NOT NULL DEFAULT '{}'::jsonb,   -- references (category_ids, item_ids) or content
  is_visible  BOOLEAN NOT NULL DEFAULT TRUE
);
CREATE INDEX menu_page_blocks_page_sort_idx ON menu_page_blocks(page_id, sort_index);
CREATE INDEX menu_page_blocks_gin ON menu_page_blocks USING GIN (config_json, data_json);

-- Pre-built templates (full pages) that owners can install and edit
CREATE TABLE menu_templates (
  id             BIGSERIAL PRIMARY KEY,
  name           TEXT NOT NULL,
  thumbnail_url  TEXT,
  blueprint_json JSONB NOT NULL,                    -- array of blocks with config/data
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE venue_template_installs (
  id           BIGSERIAL PRIMARY KEY,
  venue_id     BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  template_id  BIGINT NOT NULL REFERENCES menu_templates(id),
  installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Versioning snapshot (optional but useful for rollback/publish workflow)
CREATE TABLE menu_versions (
  id            BIGSERIAL PRIMARY KEY,
  venue_id      BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  page_snapshot JSONB NOT NULL,                     -- entire page/blocks serialized
  status        page_status NOT NULL DEFAULT 'draft',
  created_by    BIGINT REFERENCES users(id),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ===========================
-- TRANSLATIONS (generic i18n)
-- ===========================
CREATE TABLE i18n_strings (
  id           BIGSERIAL PRIMARY KEY,
  entity_type  TEXT NOT NULL,                       -- 'menu_items','menu_categories','menu_page_blocks', etc
  entity_id    BIGINT NOT NULL,
  field        TEXT NOT NULL,                       -- 'name','description','title','html', etc
  lang         TEXT NOT NULL,                       -- 'en','ar','ku', etc
  value        TEXT NOT NULL,
  UNIQUE(entity_type, entity_id, field, lang)
);

-- ===========================
-- ORDERS & PAYMENTS
-- ===========================
CREATE TABLE orders (
  id            BIGSERIAL PRIMARY KEY,
  venue_id      BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  user_id       BIGINT REFERENCES users(id),
  channel       channel_type NOT NULL DEFAULT 'public_link',
  table_no      TEXT,
  status        order_status NOT NULL DEFAULT 'pending',
  subtotal      NUMERIC(12,2) NOT NULL DEFAULT 0,
  discounts     NUMERIC(12,2) NOT NULL DEFAULT 0,
  service_fee   NUMERIC(12,2) NOT NULL DEFAULT 0,
  tax_total     NUMERIC(12,2) NOT NULL DEFAULT 0,
  total         NUMERIC(12,2) NOT NULL DEFAULT 0,
  currency      TEXT NOT NULL DEFAULT 'IQD',
  notes         TEXT,
  dynamic_pricing_applied JSONB DEFAULT '[]'::jsonb, -- audit trail of price rules applied
  upsells_applied        JSONB DEFAULT '[]'::jsonb, -- which upsells accepted
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX orders_venue_status_idx ON orders(venue_id, status, created_at DESC);

CREATE TABLE order_items (
  id            BIGSERIAL PRIMARY KEY,
  order_id      BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  item_id       BIGINT REFERENCES menu_items(id),
  variant_id    BIGINT REFERENCES item_variants(id),
  name_snapshot TEXT NOT NULL,                      -- denormalized for history
  qty           INT NOT NULL DEFAULT 1,
  unit_price    NUMERIC(12,2) NOT NULL,             -- final price per unit (after dynamic pricing)
  addons_json   JSONB NOT NULL DEFAULT '[]'::jsonb, -- [{name, price_delta}]
  line_total    NUMERIC(12,2) NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE payments (
  id            BIGSERIAL PRIMARY KEY,
  order_id      BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  method        payment_method NOT NULL,
  status        payment_status NOT NULL DEFAULT 'pending',
  amount        NUMERIC(12,2) NOT NULL,
  currency      TEXT NOT NULL DEFAULT 'IQD',
  stripe_payment_intent_id TEXT,
  external_ref  TEXT,                                -- receipt no., etc
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX payments_order_idx ON payments(order_id);

-- ===========================
-- AI-DRIVEN UPSELL RULES
-- ===========================
CREATE TABLE upsell_rules (
  id            BIGSERIAL PRIMARY KEY,
  venue_id      BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  -- scope: trigger by item/category/venue-wide
  trigger_scope rule_scope NOT NULL DEFAULT 'item',
  trigger_id    BIGINT,                              -- FK resolved in app (item_id/category_id) when relevant
  suggestion_set_json JSONB NOT NULL,                -- e.g., [{item_id, name, price_delta, bundle_price}]
  conditions_json JSONB NOT NULL DEFAULT '{}'::jsonb,-- e.g., min_cart_total, time windows, weekdays
  priority      INT NOT NULL DEFAULT 100,
  is_active     BOOLEAN NOT NULL DEFAULT TRUE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX upsell_rules_venue_active_idx ON upsell_rules(venue_id, is_active, priority);
CREATE INDEX upsell_rules_gin ON upsell_rules USING GIN (conditions_json, suggestion_set_json);

-- Track impressions/accepts for learning/optimization
CREATE TABLE upsell_stats (
  id          BIGSERIAL PRIMARY KEY,
  rule_id     BIGINT NOT NULL REFERENCES upsell_rules(id) ON DELETE CASCADE,
  impressions BIGINT NOT NULL DEFAULT 0,
  accepts     BIGINT NOT NULL DEFAULT 0,
  period_start DATE NOT NULL,
  period_end   DATE NOT NULL,
  UNIQUE(rule_id, period_start, period_end)
);

-- ===========================
-- DYNAMIC PRICING RULES
-- ===========================
CREATE TABLE pricing_rules (
  id            BIGSERIAL PRIMARY KEY,
  venue_id      BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  scope         rule_scope NOT NULL DEFAULT 'item',   -- venue/category/item
  scope_id      BIGINT,                               -- when category/item scope
  value_type    rule_value_type NOT NULL,             -- percent or fixed
  value         NUMERIC(12,2) NOT NULL,               -- +10% or +500 IQD etc.
  conditions_json JSONB NOT NULL DEFAULT '{}'::jsonb, -- {time_windows, weekdays, inventory_threshold, event_tags}
  priority      INT NOT NULL DEFAULT 100,
  is_active     BOOLEAN NOT NULL DEFAULT TRUE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX pricing_rules_venue_active_idx ON pricing_rules(venue_id, is_active, priority);
CREATE INDEX pricing_rules_gin ON pricing_rules USING GIN (conditions_json);

-- Optional inventory hook for pricing conditions
CREATE TABLE inventory_items (
  id            BIGSERIAL PRIMARY KEY,
  venue_id      BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  sku           TEXT UNIQUE,
  name          TEXT NOT NULL,
  qty_on_hand   INT NOT NULL DEFAULT 0,
  reorder_level INT NOT NULL DEFAULT 0,
  linked_item_id BIGINT REFERENCES menu_items(id),    -- if tied to a menu item
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ===========================
-- LOYALTY
-- ===========================
CREATE TABLE loyalty_settings (
  venue_id     BIGINT PRIMARY KEY REFERENCES venues(id) ON DELETE CASCADE,
  earn_rate    NUMERIC(12,4) NOT NULL DEFAULT 0.0,    -- points per 1 currency unit
  redeem_rate  NUMERIC(12,4) NOT NULL DEFAULT 0.0,    -- currency per 1 point
  min_redeem   INT NOT NULL DEFAULT 0,
  expiry_days  INT,
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE loyalty_accounts (
  id          BIGSERIAL PRIMARY KEY,
  user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  venue_id    BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  balance     INT NOT NULL DEFAULT 0,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(user_id, venue_id)
);

CREATE TABLE loyalty_transactions (
  id            BIGSERIAL PRIMARY KEY,
  account_id    BIGINT NOT NULL REFERENCES loyalty_accounts(id) ON DELETE CASCADE,
  type          loyalty_tx_type NOT NULL,
  points        INT NOT NULL,
  order_id      BIGINT REFERENCES orders(id),
  metadata      JSONB DEFAULT '{}'::jsonb,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX loyalty_tx_account_idx ON loyalty_transactions(account_id, created_at DESC);

-- ===========================
-- FEEDBACK & REVIEWS
-- ===========================
CREATE TABLE feedback (
  id           BIGSERIAL PRIMARY KEY,
  venue_id     BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  order_id     BIGINT REFERENCES orders(id) ON DELETE SET NULL,
  user_id      BIGINT REFERENCES users(id) ON DELETE SET NULL,
  rating       INT NOT NULL CHECK (rating BETWEEN 1 AND 5),
  note         TEXT,
  forwarded_to_google BOOLEAN NOT NULL DEFAULT FALSE,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX feedback_venue_idx ON feedback(venue_id, created_at DESC);

-- ===========================
-- SUBSCRIPTIONS, BILLING, TRIALS
-- ===========================
CREATE TABLE subscriptions (
  id               BIGSERIAL PRIMARY KEY,
  venue_id         BIGINT NOT NULL UNIQUE REFERENCES venues(id) ON DELETE CASCADE,
  plan             subscription_plan NOT NULL DEFAULT 'free',
  status           subscription_status NOT NULL DEFAULT 'active',
  activation_source TEXT,                               -- 'stripe' or 'cash'
  trial_end_at     TIMESTAMPTZ,
  renews_at        TIMESTAMPTZ,
  updated_by_admin_id BIGINT REFERENCES users(id),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE billing_history (
  id              BIGSERIAL PRIMARY KEY,
  venue_id        BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  subscription_id BIGINT NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
  method          payment_method NOT NULL,
  amount          NUMERIC(12,2) NOT NULL,
  currency        TEXT NOT NULL DEFAULT 'IQD',
  period_start    DATE NOT NULL,
  period_end      DATE NOT NULL,
  stripe_invoice_id TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX billing_history_venue_idx ON billing_history(venue_id, period_start DESC);

-- ===========================
-- ANALYTICS (materialized views optional)
-- ===========================
-- Lightweight aggregates table (daily rollups) to avoid heavy queries.
CREATE TABLE analytics_daily (
  id           BIGSERIAL PRIMARY KEY,
  venue_id     BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  day          DATE NOT NULL,
  orders_count INT  NOT NULL DEFAULT 0,
  revenue      NUMERIC(14,2) NOT NULL DEFAULT 0,
  new_customers INT NOT NULL DEFAULT 0,
  repeat_customers INT NOT NULL DEFAULT 0,
  top_items_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  UNIQUE(venue_id, day)
);

-- ===========================
-- AUDIT LOGS (important for admin toggles/trials/cash-activations)
-- ===========================
CREATE TABLE audit_logs (
  id            BIGSERIAL PRIMARY KEY,
  actor_user_id BIGINT REFERENCES users(id),
  venue_id      BIGINT REFERENCES venues(id),
  action        TEXT NOT NULL,                        -- 'activate_premium', 'enable_trial', 'update_menu', etc.
  entity_type   TEXT,
  entity_id     BIGINT,
  before_json   JSONB,
  after_json    JSONB,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX audit_logs_venue_action_idx ON audit_logs(venue_id, action, created_at DESC);
