# LeafWash 鲜切叶菜清洗消毒转封装前联检闭环

## 项目目标

LeafWash 是面向鲜切叶菜加工厂质控班组的单节点 Go HTTP 后端与浏览器联检页面，用于管理一次叶菜批次从种植基地批次、采收筐封签、预冷批号、切分线、清洗槽、消毒液配方、采样时点、漂洗水盲码、ATP 点位、培养板孔位、离心沥水时隙和复核人员名单锁定，到双人投料确认、消毒曲线采集、ATP 覆盖、微生物核验、理化复测、异常复判、独立复核，最终形成可转封装、已转封装、卫生隔离或已取消的唯一结论。SQLite WAL 持久化目录、任务聚合、资源占用登记、证据版本链、适配器调用、幂等结果、审计事件和终局凭据，支持进程重启后的确定恢复。规模规划为 23-27 个生产 Go 文件、约 2400-2700 行有效生产 Go 代码，覆盖 catalog、task、lease、evidence、arbiter、store、api、cmd、webembed 等包；前端为真实联检控制台，通过 HTTP/JSON 接入 Go 后端，包含 package.json、锁文件和固定构建脚本。

## 端到端业务流程

1. 创建并锁定联检任务：冻结基地批次、采收筐封签、预冷批号、切分线、清洗槽、配方摘要、余氯/ORP/pH/浊度/温度阈值、采样时点、盲码、ATP 点位、培养板孔位、沥水时隙、复核候选人员和任务代次。
2. 双人投料确认：两名不同合格人员分别确认基地批次、封签和切分线；相同操作号同内容重试返回既有结果，内容冲突返回稳定错误码。
3. 并发占用裁定：清洗槽、培养板孔位、离心沥水时隙和盲码在开放任务中独占；并发锁定、换槽和启动通过事务一次性成功或失败。
4. 消毒与理化采集：按锁定时点接收余氯、ORP、pH、温度、浊度读数，使用整数和定点规则验证格式、范围、重复时点和覆盖完整性。
5. ATP 与微生物证据闭合：ATP 点位提交不可覆盖版本；揭盲后核验菌落培养板读数、稀释倍数和样本体积，派生计算采用固定舍入并检测溢出。
6. 异常复判与独立复核：出现余氯断点、ATP 超限、菌落疑阳或漂洗水理化越界时，只允许创建一个当前代次复判证据；两名不同合格人员独立复核后进入终局竞争。

## 核心组件与职责

1. 叶菜原料与清洗规则目录：维护虚构基地批次、采收筐封签、预冷批号、切分线、清洗槽、配方修订、检测阈值、人员资质、ATP 点位模板和培养板孔位模板。
2. 清洗联检任务聚合：实现状态机、任务代次、操作号幂等、锁定快照、投料确认、阶段推进和终态屏障。
3. 槽位/板孔/沥水时隙租约登记册：用 SQLite 事务管理清洗槽、培养板孔位、离心沥水时隙和采样盲码的开放期独占占用，并支持重启恢复。
4. 消毒曲线与拭子采集记录册：记录余氯/ORP/pH/温度/浊度覆盖格、ATP 点位版本链、仪器调用尝试和待重试状态。
5. 微生物复判及终局仲裁器：处理菌落读数换算、异常复判代次隔离、独立复核资格检查，以及可转封装、已转封装、卫生隔离、已取消的单写裁定。
6. Go HTTP API 与嵌入式前端：提供 JSON API、健康检查、任务详情、操作提交、故障脚本接口和静态资源嵌入；前端展示锁定快照、占用状态、覆盖进度、异常复判和终局结果。

## 领域规则与不变量

1. 状态只能按待锁定、待投料确认、槽位占用中、消毒曲线采集中、ATP 覆盖中、微生物核验中、理化复测中、待独立复核、可转封装、已转封装、卫生隔离、已取消推进，终态拒绝任何普通写入。
2. 叶菜批次与采样盲码为一次性映射；盲码不得重复，不得在允许阶段前揭示，揭示失败不得写入任何部分结果。
3. 配方修订以锁定摘要为准；目录中更新后的修订不能被旧任务冒用，锁定时发现陈旧修订必须拒绝。
4. 同一开放任务代次内，清洗槽、培养板孔位和离心沥水时隙占用具有租约边界；失败事务不得留下部分占用。
5. 余氯使用整数毫克每升乘以 100，ORP 使用整数毫伏，pH/温度/浊度使用固定小数位整数表示；错误位数、负值、重复时点和缺失时点均拒绝。
6. 余氯衰减斜率按相邻锁定时点计算，边界值按小于等于阈值合格；断点或超阈触发当前代次复判。
7. ATP 相对光单位、菌落数、稀释倍数和样本体积计算检查长度、符号、除零和 int64 溢出；派生阈值比较采用半远离零舍入到固定精度。
8. 复核人员不得与投料确认人员角色重合；两个独立复核必须来自不同合格人员，且匹配当前任务代次。

## 数据模型与持久化

1. CatalogBaseLot：base_lot_id、crop_name、field_code、harvest_date、allowed_seals、precool_lots、active。
2. WashRuleRevision：formula_id、revision、summary_hash、chlorine_min_x100、chlorine_slope_max_x100、orp_min_mv、ph_min_x100、ph_max_x100、temperature_max_x100、turbidity_max_x100、atp_max_rlu、colony_max_cfu_x100ml、effective_from。
3. InspectionTask：task_id、generation、state、locked_snapshot_json、base_lot_id、seal_id、precool_lot、cut_line_id、wash_tank_id、formula_revision、final_result、final_credential、created_at_logic、updated_at_logic。
4. LeaseRecord：lease_id、resource_type、resource_key、task_id、generation、status、acquired_at_logic、released_at_logic，resource_type 覆盖清洗槽、板孔、沥水时隙和盲码。
5. IdempotencyRecord：operation_no、task_id、generation、operation_kind、request_hash、response_code、response_body_json。
6. CoverageSample：task_id、generation、sample_time, chlorine_x100、orp_mv、ph_x100、temperature_x100、turbidity_x100、source_call_id、valid。
7. EvidenceVersion：evidence_id、task_id、generation、kind、blind_code、point_code、plate_well、version_no、raw_json、derived_json、accepted、created_at_logic。
8. AdapterCall：call_id、adapter_kind、task_id、generation、target_key、attempt_no、script_step、status、error_code、next_retry_logic、request_json、response_json。
9. ReviewDecision：review_id、task_id、generation、reviewer_id、decision、reason_code、created_at_logic。
10. AuditEvent：event_id、task_id、generation、actor_id、event_type、reason_code、details_json、logic_time。

## 公开接口

1. POST /api/tasks/lock：锁定联检任务并原子获取所需占用；返回任务快照、代次和初始状态。
2. POST /api/tasks/{id}/feed-confirmations：提交双人投料确认，校验封签、人员和操作号幂等。
3. POST /api/tasks/{id}/curve-samples：提交某个锁定采样时点的余氯/ORP/pH/温度/浊度读数。
4. POST /api/tasks/{id}/atp-swabs：提交 ATP 点位读数版本，返回覆盖进度和异常标记。
5. POST /api/tasks/{id}/microbiology-readings：揭盲并提交培养板孔位读数、稀释倍数和样本体积，生成固定规则派生值。
6. POST /api/tasks/{id}/rechecks：提交当前代次异常点位复判证据。
7. POST /api/tasks/{id}/reviews：提交独立复核决定。
8. POST /api/tasks/{id}/finalize：竞争生成可转封装、已转封装、卫生隔离或已取消的唯一终局；GET /api/tasks/{id} 返回完整快照；前端通过这些接口呈现并操作单个闭环。

## 失败边界

1. 所有写接口在单个 SQLite 事务内完成状态校验、幂等校验、占用更新、证据写入和审计记录；失败时回滚到请求前状态。
2. 拒绝响应包含稳定 error_code 和 reasons，reasons 按 base_lot_id、wash_tank_id、sample_time、blind_code、point_code、plate_well 排序。
3. 余氯仪、ATP 读数器和培养箱适配器的拒绝、断连、超时、格式错误只写入 AdapterCall 待重试记录，不写入合格证据，不释放占用。
4. 旧代次迟到读数、复判、复核和终局请求保留为可审计拒绝事件，不覆盖当前代次证据，不改变结论。
5. 操作号相同且内容一致时返回原响应；操作号相同但内容不同返回 IDEMPOTENCY_CONFLICT。
6. 菌落换算、定点解析、阈值派生和斜率计算任一步失败，均不写入有效覆盖格或派生证据。
7. 进程重启后从 SQLite 恢复开放任务、占用、待重试调用、幂等记录和当前状态；逻辑时钟由持久化序列推进，测试可确定。

## 验收标准

1. 锁定时固定基地批次、采收筐封签、预冷批号、切分线、清洗槽、消毒液配方摘要、采样时点、盲码、ATP 点位、培养板孔位、沥水时隙、余氯/ORP/pH/浊度/温度阈值和任务代次。
2. 每个清洗槽、培养板孔位、离心沥水时隙和采样盲码在开放任务中只能被一个有效任务占用，并发锁定、换槽或启动必须原子裁定。
3. 投料确认、曲线采集、ATP 提交、揭盲与复核必须匹配当前任务代次；相同操作号同内容重试幂等，内容冲突报错，错误封签或陈旧配方不得推进状态。
4. 消毒曲线必须覆盖锁定采样时点；余氯、ORP、pH、温度、浊度和补测规则均使用确定整数/定点规则，非法读数不得写入有效覆盖格。
5. ATP 相对光单位、菌落数、稀释倍数和样本体积计算必须检查长度、符号、除零和溢出；阈值比较采用固定舍入规则，算术失败不得写入派生证据。
6. 余氯仪、ATP 读数器或培养箱适配器拒绝、断连、超时、格式错误只能形成可审计待重试调用；重试对象、次数和逻辑时间可确定，不得伪造合格结果或提前释放占用。
7. 出现余氯断点、ATP 超限、菌落疑阳或漂洗水理化越界时只能创建一个带代次的复判证据；复判须覆盖受影响时点、盲码、点位和板孔，旧代次迟到读数保持不可覆盖且不影响当前结论。
8. 全部消毒曲线、ATP 点位、微生物与理化证据闭合并满足锁定阈值，且由两名不同合格人员独立复核后，竞争成功者才可生成唯一转封装凭据；并发转封装、卫生隔离与取消只能形成一个终局，终态后的读数、复判、复核或普通操作一律被拒绝且不改变状态。

## 确定性测试场景

1. lock_test.go：成功锁定完整叶菜批次快照；基地批次与封签错配拒绝；陈旧配方修订拒绝且无占用残留。
2. lease_test.go：两个任务并发竞争同一清洗槽只成功一个；培养板孔位与沥水时隙组合占用失败时全量回滚；盲码重复或提前揭示拒绝。
3. idempotency_test.go：投料确认同操作号同内容重试返回同一结果；同操作号不同内容返回 IDEMPOTENCY_CONFLICT；旧代次提交被拒绝。
4. coverage_test.go：锁定采样时点全覆盖后推进；缺失和重复时点拒绝；余氯衰减斜率边界按固定规则判定。
5. evidence_math_test.go：pH/温度/浊度小数位错误拒绝；ATP 超限触发复判；菌落换算除零、负值和溢出均不写入派生证据。
6. adapter_retry_test.go：余氯仪断连、ATP 读数器格式错误、培养箱超时生成确定待重试调用；重试次数和逻辑时间精确断言。
7. recheck_generation_test.go：异常点位只能创建一个当前代次复判；旧代次迟到读数不可覆盖；受影响时点、盲码、点位和板孔必须完整覆盖。
8. finality_test.go：两名不同合格人员复核后并发终局只有一个成功；投料人与复核人重合拒绝；终态后普通写入与读数提交均拒绝且状态不变。

## 组件追踪关系

1. 叶菜原料与清洗规则目录覆盖锁定快照、封签匹配、配方修订、阈值和人员资格规则。
2. 清洗联检任务聚合覆盖状态模型、任务代次、投料确认、幂等冲突和终态屏障。
3. 槽位/板孔/沥水时隙租约登记册覆盖并发占用、盲码独占、事务回滚和重启恢复。
4. 消毒曲线与拭子采集记录册覆盖采样时点覆盖格、定点读数、ATP 版本链和适配器失败重试。
5. 微生物复判及终局仲裁器覆盖菌落换算、异常复判代次隔离、独立复核和唯一结论。
6. Go HTTP API 与嵌入式前端覆盖可运行服务、浏览器联检页面、确定构建和公开测试入口。

## 独特性

项目聚焦鲜切叶菜清洗消毒转封装前的质控闭环，以叶菜批次、采样盲码、清洗槽、离心沥水时隙、消毒曲线、ATP 拭子、培养板孔位和独立复核形成一个连贯流程；核心复杂度来自食品加工现场的并发占用、不可覆盖证据、仪器故障重试、定点读数规则和终局单写裁定。
