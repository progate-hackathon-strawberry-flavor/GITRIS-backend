# **GITRIS-backend**

> GitHub * TETRIS

## **1\. プロジェクト概要**

このリポジトリは、GitHubのContribution Graph（草）をテトリミノデッキとして編成するゲーム「GITRIS」のバックエンドを管理します。
Go言語で構築されており、主に
- GitHub APIからのデータ取得
- Supabase (PostgreSQL) へのデータ永続化
- TETRIS対戦のWebSocket処理
を担当します。

## **2\. ディレクトリ構成**

.  
├── cmd                 \# アプリケーションのエントリポイント  
│   └── api             \# メインのAPIアプリケーション  
│       └── main.go  
├── generate\_jwt.go     \# JWT生成に関するユーティリティ (既存)  
├── go.mod              \# Goモジュール定義  
├── go.sum              \# Goモジュールチェックサム  
├── internal            \# 内部ロジック（アプリケーション固有のコード）  
│   ├── handlers        \# HTTPリクエストのハンドラー  
│   │   └── contribution\_handler.go  
│   └── services        \# ビジネスロジック、外部サービス連携、DB操作  
│       ├── github\_service.go  
│       └── database\_service.go  
└── README.md           \# このファイル

## **3\. 使用技術**

* **Go言語**: バックエンドの主要言語  
* **Gorilla Mux**: HTTPルーティング  
* **Supabase (PostgreSQL)**: データベース  
* **GitHub GraphQL API**: GitHubデータ取得  
* **joho/godotenv**: 環境変数管理  
* **lib/pq**: PostgreSQLドライバー

## **4\. 環境変数**

.env ファイルをプロジェクトのルートディレクトリに作成し、以下の環境変数を設定してください。

APP\_ENV=development  
GITHUB\_TOKEN=your\_github\_personal\_access\_token\_here  
DATABASE\_URL=postgres://your\_username:your\_password@your\_supabase\_host:5432/your\_database\_name?sslmode=require  
PORT=8080

* APP\_ENV: production 以外の場合、.env ファイルをロードします。  
* GITHUB\_TOKEN: read:user スコープを持つGitHub Personal Access Token（PAT）。GitHub APIのレート制限を回避するために必要です。  
* DATABASE\_URL: Supabaseプロジェクトのデータベース接続文字列。Supabaseダッシュボードの \[Project Settings\] \-\> \[Database\] から取得できます。パスワードを忘れずに置き換えてください。  
* PORT: サーバーがリッスンするポート。指定しない場合、デフォルトで8080が使用されます。

## **5\. セットアップ**

### **5.1. Goモジュールの初期化と依存関係のインストール**

プロジェクトのルートディレクトリで以下を実行します。

go mod tidy

### **5.2. Supabaseデータベースのセットアップ**

以下のテーブルがSupabaseのPostgreSQLデータベースに存在することを確認してください。

**users テーブル** (例、GITRISプロジェクトのスキーマに基づく)

CREATE TABLE users (  
    id UUID PRIMARY KEY DEFAULT gen\_random\_uuid(),  
    github\_id BIGINT UNIQUE NOT NULL,  
    user\_name VARCHAR(255) NOT NULL,  
    icon\_url TEXT,  
    created\_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()  
);

**contribution\_data テーブル**

CREATE TABLE contribution\_data (  
    id UUID PRIMARY KEY DEFAULT gen\_random\_uuid(),  
    user\_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,  
    date DATE NOT NULL,  
    contribution\_count INTEGER NOT NULL,  
    created\_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),  
    UNIQUE(user\_id, date)  
);

**重要**: contribution\_data.user\_id は users.id を参照しているため、users テーブルにテスト用のユーザーデータ（UUIDとGitHubユーザー名）が少なくとも1つ存在する必要があります。

## **6\. API エンドポイント**

### **6.1. GitHub Contribution データ**

* **GET /api/contributions/{userID}**  
  * **説明**: データベースに保存されている、指定されたuserID（UUID形式）に紐づくGitHubの日ごとのContributionデータを取得します。  
  * **メソッド**: GET  
  * **パスパラメータ**: {userID} (Supabaseのusersテーブルに存在するUUID)  
  * **例**: http://localhost:8080/api/contributions/f47ac10b-58cc-4372-a567-0e02b2c3d4e5  
  * **注意**: このエンドポイントはデータベースからデータを読み取るだけで、GitHub APIへのリクエストは行いません。  
* **POST /api/contributions/refresh/{userID}**  
  * **説明**: GitHub APIから最新の日ごとのContributionデータを取得し、データベースを更新（既存データを削除し、新しいデータを挿入）します。  
  * **メソッド**: POST  
  * **パスパラメータ**: {userID} (Supabaseのusersテーブルに存在するUUID)  
  * **例**: curl \-X POST http://localhost:8080/api/contributions/refresh/f47ac10b-58cc-4372-a567-0e02b2c3d4e5  
  * **注意**: このエンドポイントはデータベースの状態を変更するため、POSTメソッドを使用します。

### **6.2. その他のエンドポイント (既存)**

* **GET /api/public**  
  * **説明**: 公開された情報を提供するテスト用エンドポイントです。  
  * **メソッド**: GET  
* **保護されたルート (/api/protected)**  
  * **説明**: handlers.AuthMiddleware を通して認証が必要なエンドポイントです。  
  * **GET /api/protected/decks**: 認証済みのユーザーのデッキデータを取得します。

## **7\. 実行方法**

cmd/api ディレクトリに移動し、以下のコマンドを実行します。

go run main.go

## **8\. デバッグのヒント**

* **connect: no route to host エラー**:  
  * ネットワーク接続の問題（学校のWi-Fiや企業ネットワークの制限など）。  
  * PCのファイアウォールが5432番ポートへのアウトバウンド接続をブロックしている。  
  * Supabaseダッシュボードの \[Project Settings\] \-\> \[Database\] の「Network Restrictions」で、あなたのIPアドレスが許可されていない。  
  * ping db.rjmewdjozhygpsozhqbi.supabase.co や telnet db.rjmewdjozhygpsozhqbi.supabase.co 5432 を実行して、ホストへの到達可能性を確認してください。  
* **pq: invalid input syntax for type uuid: "..." エラー**:  
  * contribution\_handler.go でuserIDとしてUUIDではない文字列を渡している可能性があります。usersテーブルに存在するUUIDを正しく指定してください。  
* **GitHub APIのレート制限エラー (403 Forbidden)**:  
  * .env にGITHUB\_TOKENが設定されていないか、無効なPATが設定されています。read:userスコープを持つ有効なPATを使用してください。  
* **main.go のログ**: サーバー起動時のログメッセージ (log.Printf) を注意深く確認し、エラーや警告がないかチェックしてください。
