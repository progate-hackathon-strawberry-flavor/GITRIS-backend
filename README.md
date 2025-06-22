# GITRIS-backend
GitHub * TETRIS

## 使用技術
<img src="https://go-skill-icons.vercel.app/api/icons?i=go,websocket,github,graphql,render" alt="Icons representing technologies: TypeScript, Hono, React, Dart, Flutter, Discord, GitHub, Supabase, and Vercel" />

## 環境変数設定

以下の環境変数を設定してください：

### 必須設定
```bash
# データベース接続URL（Supabase等）
DATABASE_URL=postgresql://username:password@host:port/database_name
```

### オプション設定
```bash
# サーバーポート（デフォルト: 8080）
PORT=8080

# サーバーホスト（デフォルト: localhost）
HOST=localhost

# GitHub Personal Access Token（コントリビューション取得用）
GITHUB_TOKEN=your_github_token

# 実行環境（development/production）
APP_ENV=development
```

### 本番環境の例
```bash
PORT=443
HOST=your-domain.com
APP_ENV=production
DATABASE_URL=postgresql://user:pass@prod-db:5432/gitris
```

## 起動方法

```bash
# 依存関係のインストール
go mod download

# サーバー起動
go run cmd/api/main.go
```

WebSocketテストクライアント: `http://localhost:8080/test_websocket_client.html`
