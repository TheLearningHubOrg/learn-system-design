/**
 * The Learning Hub - System Design & Distributed Systems Architecture Roadmap
 */

export const SYSTEM_DESIGN_STAGES = [
  {
    stageNumber: 1,
    title: "Foundations of Distributed Systems & Scalability",
    domainFocus: "Vertical vs Horizontal scaling, Latency vs Throughput, CAP theorem, PACELC theorem, SLA/SLO/SLI 9s",
    completed: true,
    keyTopics: ["CAP Theorem Trade-offs", "PACELC Invariants", "SLA 9s Uptime Calculus", "Single Point of Failure (SPOF) Removal"]
  },
  {
    stageNumber: 2,
    title: "Load Balancing, Networking & Content Delivery",
    domainFocus: "L4 vs L7 load balancing, DNS routing, Anycast, Consistent Hashing rings, CDN edge caching",
    completed: true,
    keyTopics: ["Consistent Hashing with Virtual Nodes", "L4 vs L7 Reverse Proxies", "Round-Robin & Least-Connections", "CDN Cache Eviction & Stale-While-Revalidate"]
  },
  {
    stageNumber: 3,
    title: "Data Partitioning, Sharding & Distributed Storage",
    domainFocus: "Vertical/Horizontal sharding, Shard keys, 2PC (Two-Phase Commit), Saga orchestration, DynamoDB LSM Trees",
    completed: true,
    keyTopics: ["Range vs Hash-based Sharding", "Distributed Transactions (2PC vs Saga)", "LSM-Trees vs B-Trees", "Replication Lag & Read Replicas"]
  },
  {
    stageNumber: 4,
    title: "Caching Strategies & Distributed Invalidation",
    domainFocus: "Cache-Aside, Read-Through, Write-Through, Write-Behind, Redis cluster sharding, Thundering Herd mitigation",
    completed: true,
    keyTopics: ["Cache-Aside Pattern", "Cache Stampede (Probabilistic Early Expiration)", "Redis Sentinel vs Cluster", "LRU / LFU Eviction Algorithms"]
  },
  {
    stageNumber: 5,
    title: "Event-Driven Architecture & Message Brokers",
    domainFocus: "Kafka commit logs, RabbitMQ AMQP, idempotency keys, dead letter queues (DLQ), outbox pattern",
    completed: true,
    keyTopics: ["Transactional Outbox Pattern", "Kafka Partition Hashing & Consumer Groups", "Exactly-Once Semantics (EOS)", "Backpressure & Circuit Breakers"]
  },
  {
    stageNumber: 6,
    title: "Distributed Coordination & Consensus Engines",
    domainFocus: "Raft consensus, Paxos, Apache ZooKeeper, etcd distributed leases, distributed lock managers (Redlock)",
    completed: true,
    keyTopics: ["Raft Leader Election & Log Replication", "Split-Brain Prevention", "Distributed Locking with Fencing Tokens", "Twitter Snowflake Unique IDs"]
  },
  {
    stageNumber: 7,
    title: "Staff System Architect Interview Handbook & Blueprints",
    domainFocus: "40+ staff scenario blueprints: TinyURL, Twitter feed, Uber geospatial dispatch, Netflix streaming CDN",
    completed: true,
    keyTopics: ["Geohash & Quadtree Spatial Indexing", "Fan-out on Write vs Read", "Rate Limiter (Token Bucket / Sliding Window)", "Disaster Recovery Active-Active Multi-Region"]
  }
];

export function getCompletedCount() {
  return SYSTEM_DESIGN_STAGES.filter(s => s.completed).length;
}

export function getProgressPercentage() {
  return Math.round((getCompletedCount() / SYSTEM_DESIGN_STAGES.length) * 100);
}
