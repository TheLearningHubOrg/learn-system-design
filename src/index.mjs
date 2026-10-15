import { ConsistentHashRing } from './distributed/consistent-hash.mjs';
import { SnowflakeIdGenerator } from './distributed/snowflake.mjs';
import { SYSTEM_DESIGN_STAGES, getCompletedCount, getProgressPercentage } from './roadmap.mjs';

console.log('======================================================');
console.log('  The Learning Hub - Distributed Systems & Architecture');
console.log('======================================================');
console.log('Paradigm: High-Availability, Fault-Tolerant Distributed Architectures');
console.log('Core Tenets: CAP Theorem, Consistent Hashing, Snowflake IDs, Consensus\n');

// 1. SLA 9s Availability Calculus
console.log('--- 1. Availability SLA 9s Annual Downtime Calculus ---');
function printSla(sla) {
  const totalMinutes = 365.25 * 24 * 60;
  const downMinutes = totalMinutes * (1 - sla / 100);
  const downHours = (downMinutes / 60).toFixed(2);
  console.log(`  SLA ${sla}%: ~${downMinutes.toFixed(1)} mins/year (${downHours} hours allowed outage)`);
}
[99.0, 99.9, 99.99, 99.999].forEach(printSla);

// 2. Consistent Hashing Demonstration
console.log('\n--- 2. Consistent Hashing Ring with 50 Virtual Nodes/Host ---');
const ring = new ConsistentHashRing(50);
ring.addNode('cache-cluster-us-east');
ring.addNode('cache-cluster-eu-west');
ring.addNode('cache-cluster-ap-south');

const userSessions = ['user:alice:session', 'user:bob:session', 'user:charlie:session', 'user:diana:session', 'user:evan:session'];
console.log('Initial Key Distribution:');
userSessions.forEach(key => {
  console.log(`  Key '${key}' -> routed to [${ring.getNode(key)}]`);
});

console.log('\nSimulating Node Failure: removing [cache-cluster-eu-west]...');
ring.removeNode('cache-cluster-eu-west');
console.log('Post-Failure Key Distribution (minimal key migration):');
userSessions.forEach(key => {
  console.log(`  Key '${key}' -> routed to [${ring.getNode(key)}]`);
});

// 3. Twitter Snowflake 64-bit ID Generation
console.log('\n--- 3. Twitter Snowflake 64-bit Monotonic ID Generator ---');
const generator = new SnowflakeIdGenerator(42n);
console.log('Generated Monotonic IDs (Machine #42):');
for (let i = 0; i < 4; i++) {
  console.log(`  ID #${i + 1}: ${generator.nextId()}`);
}

// 4. 7-Stage Pedagogical Roadmap
console.log('\n--- 4. 7-Stage System Design Masterclass Roadmap ---');
for (const stage of SYSTEM_DESIGN_STAGES) {
  const icon = stage.completed ? '[OK]' : '[..]';
  console.log(` ${icon} Stage ${stage.stageNumber}: ${stage.title}`);
  console.log(`      Focus: ${stage.domainFocus}`);
  console.log(`      Topics: ${stage.keyTopics.join(', ')}`);
}

const completed = getCompletedCount();
const percent = getProgressPercentage();
console.log('------------------------------------------------------');
console.log(`Progress: ${completed}/7 Stages Complete (${percent}%)`);
console.log('======================================================\n');
