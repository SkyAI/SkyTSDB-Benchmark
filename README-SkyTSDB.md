# 改造说明
- 基于 `InfluxDB Benchmark` 中 `OpenTSDB` 测试 `Case` 改造而来
- 适配了 `SkyTSDB Benchmark` 的部分测试场景
    1. 适配支持 `SkyTSDB` 最大 8 个 `Tag` 数的支持
    2. 为了和现有的真实环境相匹配，生成数据间隔从 `10s` 改为 `1s`
    3. 生成数据添加了 `Metric` 后缀选项，目的是为了生成更多的数据
    4. 增加了简单的压缩比测试工具
    5. 增加官方 `README` 中文文档
    
# SkyTSDB Benchmark Case
## Five phases

1. data generation（数据产生）
2. data loading（数据加载）
3. query generation（查询产生）
4. query execution（查询执行）
5. query validation（查询验证）

### Phase 1 Data generation（数据产生）
#### 部分重要参数说明及示例
- 生成指定 的时间区间

```bash
bulk_data_gen --use-case=devops --scale-var=1 --format=skytsdb --timestamp-start="2018-12-17T00:00:00.00+08:00" --timestamp-end="2018-12-17T00:00:01.00+08:00" > test.data
```
- 生成数据量计算公式（当前为每隔1秒产生数据）

```bash
# 1秒产生的数据量
1 * 101 = 101点
# 1天产生的数据量
24 * 60 * 60 * 101 = 8726400点
```
-  `-scale-var`

> Scaling variable specific to the use case. (default 1)

以上官方给的说明，可理解为模拟系统集群的个数，实测改变量影响 `hostname` 的 `tag`，值为1生成的 `tag` 为 `"hostname":"host_0"`，值为2数据量加倍，生成`tag` 为 `"hostname":"host_0"`和 `"hostname":"host_1"`的点

- 生成待测试的压缩数据

```shell
bulk_data_gen --use-case=devops --scale-var=1 --format=skytsdb | gzip > skytsdb_bulk_records__usecase_devops__scalevar_1.gz
```
生成数据成功结果如下

```
using random seed 558276000
2018/11/28 16:02:54 Written 77760 points, 872640 values, took 5.197140 seconds
```
- 验证测试数据是否正确(查看生成数据的前3行)

```shell
cat skytsdb_bulk_records__usecase_devops__scalevar_1.gz | gunzip | head -n 3 
```
### Phase 2 Data loading（装载数据）
- 装载数据

```shell
cat skytsdb_bulk_records__usecase_devops__scalevar_1.gz | gunzip | bulk_load_skytsdb -urls http://127.0.0.1:8080  --batch-size=5000 --workers=1
```
- 结果如下

```
daemon URLs: [http://127.0.0.1:8080]
backoffs took a total of 0.000000sec of runtime
loaded 872640 items in 9.115370sec with 2 workers (mean values rate 95732.810946/sec)
```
可看到平均吞吐量为 95732.810946 datapoint/sec
### Phase 3: Query generation（生成查询）
- 生成查询

```shell
bulk_query_gen -query-type "1-host-1-hr" --use-case=devops --queries=1000 --format=skytsdb | gzip > devops_1_host_1_hr_queries_1000_skytsdb.gz
```
- 查询生成成功后结果如下

```shell
using random seed 997778000
OpenTSDB max cpu, rand    1 hosts, rand 1h0m0s by 1m: 1000 points
```

### Phase 4: Query execution（执行查询测试）
- 查询测试

```shell
cat devops_1_host_1_hr_queries_1000_skytsdb.gz | gunzip | query_benchmarker_skytsdb -urls http://127.0.0.1:8080/datanode -workers=2
```

- 测试结果

```shell
daemon URLs: [http://127.0.0.1:8080/datanode]
after 100 queries with 2 workers:
OpenTSDB max cpu, rand    1 hosts, rand 1h0m0s by 1m : min:    36.35ms (  27.51/sec), mean:    61.34ms (  16.30/sec), max:  151.58ms (  6.60/sec), count:      100, sum:   6.1sec 
all queries                                          : min:    36.35ms (  27.51/sec), mean:    61.34ms (  16.30/sec), max:  151.58ms (  6.60/sec), count:      100, sum:   6.1sec 

......

after 900 queries with 2 workers:
OpenTSDB max cpu, rand    1 hosts, rand 1h0m0s by 1m : min:    30.77ms (  32.49/sec), mean:    59.09ms (  16.92/sec), max:  273.45ms (  3.66/sec), count:      900, sum:  53.2sec 
all queries                                          : min:    30.77ms (  32.49/sec), mean:    59.09ms (  16.92/sec), max:  273.45ms (  3.66/sec), count:      900, sum:  53.2sec 

after 1000 queries with 2 workers:
OpenTSDB max cpu, rand    1 hosts, rand 1h0m0s by 1m : min:    30.77ms (  32.49/sec), mean:    59.27ms (  16.87/sec), max:  273.45ms (  3.66/sec), count:     1000, sum:  59.3sec 
all queries                                          : min:    30.77ms (  32.49/sec), mean:    59.27ms (  16.87/sec), max:  273.45ms (  3.66/sec), count:     1000, sum:  59.3sec 

run complete after 1000 queries with 2 workers:
OpenTSDB max cpu, rand    1 hosts, rand 1h0m0s by 1m : min:    30.77ms (  32.49/sec), mean:    59.27ms (  16.87/sec), max:  273.45ms (  3.66/sec), count:     1000, sum:  59.3sec 
all queries                                          : min:    30.77ms (  32.49/sec), mean:    59.27ms (  16.87/sec), max:  273.45ms (  3.66/sec), count:     1000, sum:  59.3sec 
```
### Phase 5: Query validation
- 查询验证（验证程序的确查询出结果）

```
cat skytsdb_1_host-1-hr-new.query | query_benchmarker_skytsdb -urls http://10.201.12.66:8080/datanode --print-interval=0 --limit=1 --workers=1 --debug=4
```
- 验证结果

```
debug:   response: {"outputs":[{"id":"a","alias":"output","dps":[[1514808000000,57.42690779501119],[1514808060000,57.47502779799193],[1514808120000,54.490095431699174],[1514808180000,49.69628967772119],[1514808240000,51.18899553386201],[1514808300000,46.68683964458759],[1514808360000,44.88780167845436],[1514808420000,43.700143667490416],[1514808480000,44.29593702674555]
```
### Phase 6: Compaction Ratio
- 记录数据库初始大小

```
compaction_ratio_benchmarker_skytsdb -urls 10.201.12.66 --step=init --path=/hbase/data
```
- 结果

```
2018/12/25 18:07:21 Write initial size success,iniSize:943805543
```
- 数据导入成功落盘后计算压缩率

```
compaction_ratio_benchmarker_skytsdb -urls 10.201.12.66:9000 --step=calc --path=/hbase/data --dataSize=325884472
```

- 结果

```
2018/12/25 18:05:52 ratio:0.0628438933
```