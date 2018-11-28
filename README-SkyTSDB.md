# 改造说明
- 基于 InfluxDB benchmark 中 OpenTSDB 测试 Case 改造而来
- 更改了部分代码用于适配 SkyTSDB
    1. 比如默认支持的 Tag 数量，因 SkyTSDB 基于 OpenTSDB 二次开发默认只支持最大 8 个 Tag。因此改造了测试 Case 最多只会生成 8 个 Tag 的测试数据
    2. SkyTSDB 默认插入数据接口不支持 gzip 压缩，因此更改了这部分代码
    3. 查询测试默认是提交 Form 表单，在转发数据的时候需要 decode 表单内容
    
# SkyTSDB Benchmark Case
- [参考wiki](http://192.168.20.14/SkyDB/SkyTSDB-Benchmark/wikis/SkyTSDB-Benchmark-Case)