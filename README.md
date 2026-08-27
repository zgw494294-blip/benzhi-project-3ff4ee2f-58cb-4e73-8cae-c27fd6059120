# strata-proof 地层关系证据校核工作台

`strata-proof` 面向田野考古团队，把探方建档、地层单位不可变修订、叠压/打破/同期关系、图一致性检查、问题整改、人工复核、证据冻结和研究凭据验真串成一条可追溯流程。工作台支持单位表格批量登记、关系最短证据路径追溯、历史检查批次对比、结构化复核清单与定向退回，以及研究凭据撤销和关联补发。服务为单进程 Go 应用，浏览器工作台与 JSON API 同源提供，本地 SQLite 保存案卷事实和连续审计记录，不需要 Node 构建链或外部系统。

## 构建与测试

要求 Go 1.23 或更高版本。

```text
go build ./cmd/server
go test ./...
```

## 运行工作台

默认仅监听高位回环地址 `127.0.0.1:19081`，数据写入当前目录的 `strata-proof.db`：

```text
go run ./cmd/server
```

浏览器访问 `http://127.0.0.1:19081/workbench`。可通过 `-addr` 指定另一个回环地址，也可让 `PORT` 提供端口号；显式 `-addr` 优先于 `PORT`。服务会拒绝非回环监听地址。

```text
go run ./cmd/server -addr=127.0.0.1:19091 -db=field.db
PORT=19092 go run ./cmd/server
```

## 完整流程 selfcheck

下面的命令使用内存 SQLite 和真实 `127.0.0.1:19081` HTTP 监听，依次完成建档、单位批量登记、关系建立与路径追溯、检查批次查询、提交复核、冻结、凭据签发、撤销、关联补发和验真，然后自行关闭退出：

```text
go run ./cmd/server -selfcheck -addr=127.0.0.1:19081
```

每个变更 API 都要求 `expectedVersion`、`actor` 和长度不少于 8 的 `idempotencyKey`。陈旧版本返回 `409`；同一幂等键的网络重试返回首次保存的原始结果。批量登记限制为 1 至 100 行并执行整批原子校验；关系路径与检查批次查询不改变案卷版本或审计序号。复核通过必须确认四类清单，退回必须定位单位、关系或最新问题；定向整改闭环后还需重新检查。冻结后业务内容不能修改，凭据撤销和补发也不会解除冻结。

主要扩展入口如下：

- `POST /api/v1/dossiers/{id}/units/batch`：原子批量登记单位。
- `GET /api/v1/dossiers/{id}/relation-path`：按 `sourceUnitId`、`targetUnitId` 追溯路径。
- `GET /api/v1/dossiers/{id}/checks`：按 `severity`、`changeType` 查询只读检查批次。
- `PATCH /api/v1/dossiers/{id}/remediations/{remediationID}`：记录定向整改处理说明。
- `POST /api/v1/dossiers/{id}/credentials/{credentialID}/revoke` 与 `/reissue`：撤销和关联补发凭据。
