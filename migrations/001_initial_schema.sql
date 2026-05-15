-- 1. UUID生成機能の有効化 (標準Postgresでgen_random_uuidを使うため)
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 2. users テーブル
-- auth.uid() への依存を削除し、IDは自動生成されるように変更
CREATE TABLE public.users (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  display_name text DEFAULT 'NO NAME'::text,
  github_id text,
  icon_url text,
  user_name text,
  CONSTRAINT users_pkey PRIMARY KEY (id)
);

-- 3. contribution_data テーブル
CREATE TABLE public.contribution_data (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL, 
  date date NOT NULL,
  contribution_count bigint NOT NULL DEFAULT 0,
  created_at timestamp with time zone DEFAULT now(),
  CONSTRAINT contribution_data_pkey PRIMARY KEY (id),
  CONSTRAINT contribution_data_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

-- 4. decks テーブル
CREATE TABLE public.decks (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL UNIQUE, 
  total_score bigint DEFAULT 0,
  created_at timestamp with time zone DEFAULT now(),
  updated_at timestamp with time zone DEFAULT now(),
  CONSTRAINT decks_pkey PRIMARY KEY (id),
  CONSTRAINT decks_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

-- 5. results テーブル
CREATE TABLE public.results (
  id bigint GENERATED ALWAYS AS IDENTITY NOT NULL,
  score bigint NOT NULL DEFAULT 0,
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  user_id uuid NOT NULL, 
  CONSTRAINT results_pkey PRIMARY KEY (id),
  CONSTRAINT results_user_id_fkey1 FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

-- 6. tetrimino_placements テーブル
CREATE TABLE public.tetrimino_placements (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  deck_id uuid NOT NULL,
  tetrimino_type varchar NOT NULL DEFAULT '',
  rotation integer NOT NULL DEFAULT 0,
  start_date date NOT NULL,
  positions jsonb NOT NULL,
  score_potential bigint NOT NULL DEFAULT 0,
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  CONSTRAINT tetrimino_placements_pkey PRIMARY KEY (id),
  CONSTRAINT tetrimino_placements_deck_id_fkey FOREIGN KEY (deck_id) REFERENCES public.decks(id) ON DELETE CASCADE
);
