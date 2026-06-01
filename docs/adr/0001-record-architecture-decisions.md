# 1. Record architecture decisions

Date: 2025-01-XX

## Status

Accepted

## Context

刚起步的项目需要一个轻量级机制来记录"为什么这么设计"，
避免几周后自己也忘了当初的决策依据。

## Decision

使用 ADR (Architecture Decision Records) 记录所有重要技术决策。
- 文件位置：`docs/adr/NNNN-title.md`
- 编号：四位数字，递增
- 模板：参考 Michael Nygard 经典格式（Context / Decision / Consequences）

## Consequences

- 优点：决策可追溯，新人（或未来的自己）能快速理解
- 缺点：需要养成"重大变更前先写 ADR"的习惯
