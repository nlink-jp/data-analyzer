# RFP: data-analyzer

> Generated: 2026-04-15
> Status: Draft

## 1. Problem Statement

大規模ログデータ（JSON/JSONL、最大10万件規模）から、自然言語の分析指示に基づいて洞察を得たい。ローカルLLM（LM Studio / Ollama）を利用するが、コンテキストウィンドウの制限によりデータ全体を一度にLLMへ投入できない。

従来のMap-Reduce方式では、Reduce時にコンテキスト情報が圧縮されすぎて分析精度が劣化する問題がある。本ツールは**スライドウィンドウ＋段階的要約方式**を採用し、前回の要約と新しいRAWデータ、蓄積されたFindingsを逐次合成しながら分析を進める。ハルシネーション対策として、すべてのFindingにRAWデータの引用を義務づける。

ターゲットユーザーは自組織のセキュリティ・運用チーム。端末操作ログやアクセスログなどの大量データから、分析観点に従った発見や洞察を自然言語レポートとして得ることを目的とする。

## 2. Functional Specification

### Commands / API Surface

3つのサブコマンドで構成：

| サブコマンド | 役割 |
|---|---|
| `prepare` | 対話的に分析プロンプトを構築し、パラメータJSONファイルを出力 |
| `analyze` | パラメータ＋入力データからスライドウィンドウ分析を実行し、構造化JSONを出力 |
| `compile` | 分析結果JSONをMarkdown/HTML等にレンダリング |

**analyze サブコマンド主要フラグ：**
- `--params <file>` — パラメータJSONファイル（またはBASE64文字列）
- `--resume <job-id>` — 中断したジョブの再開
- `--output <file>` — 出力先ファイル（省略時stdout）

**compile サブコマンド主要フラグ：**
- `--format <md|html|both>` — 出力形式
- `--output <file>` — 出力先ファイル（省略時stdout）

**prepare サブコマンド主要フラグ：**
- `--output <file>` — パラメータJSON出力先

### Input / Output

**入力：**
- JSON（配列形式）またはJSONL（1行1オブジェクト）
- ファイル指定、ディレクトリ指定（バッチ処理）、stdin対応

**出力：**
- `analyze` → 構造化JSON（AnalysisResult：要約、Findings、引用情報）
- `compile` → Markdown / HTML レポート
- 進捗情報はstderrに表示

### Configuration

3層設定（既存慣例準拠）：
1. デフォルト値（コンパイル時）
2. 設定ファイル: `~/.config/data-analyzer/config.toml`
3. 環境変数: `DATA_ANALYZER_API_ENDPOINT`, `DATA_ANALYZER_API_MODEL`, etc.
4. CLIフラグ（最優先）

```toml
[api]
endpoint = "http://localhost:1234/v1"
model = "google/gemma-4-26b-a4b"
api_key = ""

[analysis]
context_limit = 131072  # 128K tokens
overlap_ratio = 0.1
max_findings = 100

[job]
temp_dir = ""  # default: os.TempDir()/data-analyzer
```

### External Dependencies

- ローカルLLM: LM Studio または Ollama（OpenAI互換API）
- Go標準ライブラリ + `nlk`（組織内共通ライブラリ）
- 外部クラウドサービスへの依存なし

## 3. Design Decisions

**言語: Go**
- macOS / Windows混在環境でのクロスプラットフォーム配布が容易（単一バイナリ）
- 既存のnlkライブラリ（guard, backoff, strip, jsonfix, validate）が利用可能
- util-seriesの既存ツール（gem-query, mail-analyzer-local等）と一貫したエコシステム

**LLM呼び出し: OpenAI互換APIを直接実装**
- net/httpで直接HTTPリクエスト（外部SDK不使用）
- 内部でモジュール化（LLMClient interface）し、テスト容易性とバックエンド追加の拡張性を確保
- nlk/backoffによるリトライ、nlk/stripによるThinkタグ除去、nlk/jsonfixによるJSON修復

**コアアルゴリズム: スライドウィンドウ＋段階的要約**
- Map-Reduceの情報損失問題を回避
- オーバーラップ付きウィンドウで境界コンテキストを保持
- Findingsの蓄積と優先度管理（高重要度を保持、低重要度をFIFO eviction）
- RAWデータ引用の義務化によるハルシネーション対策

**既存ツールとの関係: 独立**
- gem-queryは検索特化、json-filterはJSON修復で役割が異なる
- mail-analyzer-localのLLMクライアントパターンを移植・汎用化

**スコープ外：** 特になし（Phase 1時点）

## 4. Development Plan

### Phase 1: Core
- プロジェクトスキャフォールド（`_wip/data-analyzer/`）
- LLMクライアントモジュール（OpenAI互換API、リトライ、nlk統合）
- 3層設定管理（config.toml + env + flags）
- JSON/JSONLリーダー（ファイル/ディレクトリ/stdin）
- トークン推定（CJK対応）
- スライドウィンドウエンジン（コアアルゴリズム）
- メモリマップ（コンテキスト予算の動的配分）
- ジョブ管理（ID、チェックポイント、中断再開、冪等性）
- `analyze`サブコマンド
- 全コアモジュールのユニットテスト

### Phase 2: Features
- `prepare`サブコマンド（対話的プロンプト構築）
- `compile`サブコマンド（Markdown出力）
- stderr進捗表示
- ディレクトリ一括入力・stdin対応
- BASE64パラメータ入力

### Phase 3: Release
- HTML出力対応（compile）
- ドキュメント整備（README.md / README.ja.md）
- CHANGELOG.md
- クロスプラットフォームビルド・リリース

各フェーズは独立してレビュー可能。

## 5. Required API Scopes / Permissions

- ローカルLLM利用のため外部クラウド認証は不要
- OpenAI互換APIのAPI Key対応は実装（ローカルAPIでは通常未使用だが、将来的なリモートAPI利用に備える）

## 6. Series Placement

Series: **util-series**
Reason: パイプフレンドリーなデータ変換・処理CLIとして、gem-query, json-filter, mail-analyzer-local等と同列に位置づけ。ローカルLLM利用ではあるが、ツールの本質はデータ分析CLIであり、lite-series（ローカルLLM対話ツール群）とは目的が異なる。

## 7. External Platform Constraints

- **モデル:** google/gemma-4-26b-a4b（256Kコンテキスト）を想定
- **設計上のコンテキスト上限:** 128Kトークン（良好な性能確保のため半分に制限）
- **エンドポイント:** 主にLM Studio（localhost:1234/v1）を想定
- **LM Studio / Ollama間のAPI実装差異:** レスポンスフォーマット対応の違いあり
- **ハードウェア依存:** レスポンス速度がローカルGPU性能に依存
- **Think mode:** mail-analyzer-localの知見より、Thinkモードは精度劣化の可能性あり。デフォルトOFFで設計

---

## Discussion Log

1. **コンテキスト制限突破の手法検討** — Map-Reduce方式の情報損失問題を議論。スライドウィンドウ＋段階的要約方式を採用。前回要約＋新RAWデータ＋蓄積Findingsを逐次合成する方式で、Reduce時のコンテキスト圧縮による精度劣化を回避。

2. **アーキテクチャの2パス構成** — 当初3段階（Compile→Analyze→Report）を検討したが、2パス構成（analyze → compile）に変更。各サブコマンドが独立実行可能なUNIX哲学に従う設計。

3. **対話的プロンプト構築** — 分析観点をそのままLLMに渡すのではなく、一度LLMで内容分析して分析に必要な情報を構造化する「コンパイル」工程が必要との判断。`prepare`サブコマンドとして対話的に実装。完成したパラメータはJSONファイルとして出力し、`analyze`時に指定。BASE64でのCLI引数渡しも対応。

4. **ジョブ管理と冪等性** — 10万件の処理は長時間を要するため、チェックポイントによる中断再開を実装。ジョブIDはタイムスタンプ＋入力ハッシュで生成。完了済みジョブは再実行をスキップ（冪等性）。内容の冪等性はLLMの非決定性により保証不可であることは許容。

5. **ハルシネーション対策** — FindingにはRAWデータのレコードインデックスによる引用を義務化。プロンプトで「[Record #N]」形式の参照を要求し、Citationとして構造化保存。

6. **言語選定** — macOS/Windows混在環境でのクロスプラットフォーム要件からGoを選定。既存のnlkライブラリ・util-seriesエコシステムとの整合性も決め手。

7. **メモリマップ設計** — 128Kトークン予算を動的に配分。Findings蓄積に伴いRAWデータ予算が縮小するが、後半はリッチなコンテキストを持つため許容。Findings肥大時は優先度ベースのFIFO evictionで対応。
