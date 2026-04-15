# data-analyzer

ローカルLLMを使用した大規模JSON/JSONLデータ分析CLI。

**スライドウィンドウ＋段階的要約方式**により、コンテキストウィンドウの制限を克服 — Map-Reduceの情報損失なし。

## 特徴

- 10万件以上のJSON/JSONLレコードをローカルLLMで分析
- オーバーラップ付きスライドウィンドウで境界コンテキストを保持
- citation検証 — すべてのFindingがソースレコードを引用し、オリジナルと照合
- チェックポイントベースの中断再開
- 冪等なジョブ実行
- 対話的パラメータ構築（ファイル入力対応）
- Markdown / HTMLレポート出力
- 出力言語制御（`--lang Japanese`）

## 必要環境

- Go 1.23+
- OpenAI互換APIを持つローカルLLM（例：[LM Studio](https://lmstudio.ai/)）
- 推奨モデル：`google/gemma-4-26b-a4b`（Think OFF）

## インストール

```bash
make build    # → dist/data-analyzer
```

## セットアップ

```bash
# 方法1: 環境変数
export DATA_ANALYZER_API_ENDPOINT="http://localhost:1234/v1"
export DATA_ANALYZER_API_MODEL="google/gemma-4-26b-a4b"

# 方法2: 設定ファイル（~/.config/data-analyzer/config.toml）
mkdir -p ~/.config/data-analyzer
cp config.example.toml ~/.config/data-analyzer/config.toml
```

## 使い方

### 1. 分析パラメータの準備

LLMの支援を受けて対話的にパラメータを構築：

```bash
# 対話モード（複数行入力対応、空行で入力確定）
data-analyzer prepare --output params.json

# サンプルデータ付き — LLMが実際のフィールド名と値を参照して提案
data-analyzer prepare --sample logs.jsonl --output params.json

# ファイルから要件＋サンプルデータ、その後対話的にリファイン
data-analyzer prepare --input requirements.txt --sample logs.jsonl --output params.json
```

または `params.json` を手動作成：

```json
{
  "perspective": "内部脅威と不正アクセスを検出する",
  "target_fields": ["user", "action", "source_ip", "timestamp"],
  "attention_points": [
    "複数回のログイン失敗",
    "権限昇格",
    "外部サービスへの大量データ転送"
  ],
  "user_findings": [],
  "lang": "Japanese"
}
```

### 2. 分析の実行

```bash
# 単一ファイル
data-analyzer analyze --params params.json logs.jsonl

# ディレクトリ（.json/.jsonlファイルを一括処理）
data-analyzer analyze --params params.json ./log_dir/

# 出力ファイル＋言語指定
data-analyzer analyze --params params.json --lang Japanese --output result.json logs.jsonl

# 中断した分析の再開
data-analyzer analyze --params params.json --resume <job-id> logs.jsonl
```

### 3. レポート生成

```bash
# Markdownを標準出力に
data-analyzer compile result.json

# HTMLレポート
data-analyzer compile --format html --output report.html result.json

# MarkdownとHTML両方
data-analyzer compile --format both --output report result.json

# 標準入力から
cat result.json | data-analyzer compile -
```

## 設定

設定の読み込み順序：デフォルト値 → 設定ファイル → 環境変数 → CLIフラグ

| 変数 | デフォルト | 説明 |
|------|-----------|------|
| `DATA_ANALYZER_API_ENDPOINT` | `http://localhost:1234/v1` | OpenAI互換APIエンドポイント |
| `DATA_ANALYZER_API_MODEL` | `google/gemma-4-26b-a4b` | モデル名 |
| `DATA_ANALYZER_API_KEY` | — | APIキー（任意） |
| `DATA_ANALYZER_CONTEXT_LIMIT` | `131072` | コンテキストウィンドウ予算（トークン数） |
| `DATA_ANALYZER_OVERLAP_RATIO` | `0.1` | ウィンドウオーバーラップ率（0.0–1.0） |
| `DATA_ANALYZER_MAX_FINDINGS` | `100` | 蓄積するFindingsの最大数 |
| `DATA_ANALYZER_MAX_RECORDS_PER_WINDOW` | `200` | ウィンドウあたりの最大レコード数（品質ガード） |
| `DATA_ANALYZER_LANG` | — | 出力言語（例：`Japanese`） |
| `DATA_ANALYZER_TEMP_DIR` | `$TMPDIR/data-analyzer` | チェックポイントディレクトリ |

## 動作原理

```
┌─────────────┐    ┌──────────────┐    ┌──────────────┐
│   prepare    │───▶│   analyze    │───▶│   compile    │
│  （対話的）   │    │（ｽﾗｲﾄﾞｳｨﾝﾄﾞｳ）│    │(md/html/both)│
└─────────────┘    └──────────────┘    └──────────────┘
   params.json        result.json       report.md/.html
```

**スライドウィンドウアルゴリズム：**

1. レコードをオーバーラップ付きウィンドウに分割（最大200件/ウィンドウ）
2. 各ウィンドウ：`[前回要約] + [蓄積Findings] + [新RAWデータ]` → LLM
3. LLMが更新された要約＋レコード引用付きの新Findingsを返却
4. citation検証：excerptの関連性を確認し、オリジナルレコード全体に差替
5. 各ウィンドウ処理後にチェックポイントを保存（中断時に再開可能）
6. 蓄積されたFindingsから最終レポートを生成

**citation検証：**

LLMからの全citationをオリジナルデータと照合：
- excerptの値がオリジナルレコードに存在するか関連性チェック
- excerptは常にオリジナルレコード全体に差替（フィールド省略なし）
- 一致しないexcerptはハルシネーションの可能性として警告
- 欠損citationはdescription中の`Record #N`参照から復旧

**メモリマップ（128Kトークン予算）：**

| セクション | 配分 |
|-----------|------|
| システムプロンプト | 〜2K（固定） |
| 前回要約 | 0→15K（成長後安定） |
| 蓄積Findings | 0→20K（成長、優先度eviction） |
| 新RAWデータ | 残り（〜86K–106K） |
| レスポンスバッファ | 〜5K（固定） |

## ライセンス

MIT
