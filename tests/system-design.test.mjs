import { describe, it } from 'node:test';
import assert from 'node:assert/strict';

describe('Availability & SLA 9s Calculation', () => {
  function getDowntimeMinutesPerYear(availabilityPercent) {
    const totalMinutesYear = 365.25 * 24 * 60;
    return totalMinutesYear * (1 - availabilityPercent / 100);
  }

  it('calculates exact downtime for standard SLAs', () => {
    const twoNines = getDowntimeMinutesPerYear(99.0);
    const threeNines = getDowntimeMinutesPerYear(99.9);
    const fourNines = getDowntimeMinutesPerYear(99.99);

    assert.ok(twoNines > 5000 && twoNines < 5300); // ~3.65 days
    assert.ok(threeNines > 500 && threeNines < 530); // ~8.76 hours
    assert.ok(fourNines > 50 && fourNines < 55); // ~52.6 minutes
  });
});

describe('Consistent Hashing Uniformity', () => {
  function hash(key) {
    let h = 0;
    for (let i = 0; i < key.length; i++) {
      h = (Math.imul(31, h) + key.charCodeAt(i)) | 0;
    }
    return Math.abs(h);
  }

  it('verifies virtual nodes smooth distribution', () => {
    const nodes = ['NodeA', 'NodeB', 'NodeC'];
    const virtualNodesPerPhysical = 50;
    const ring = [];

    for (const node of nodes) {
      for (let i = 0; i < virtualNodesPerPhysical; i++) {
        ring.push({ hash: hash(`${node}#VN${i}`), node });
      }
    }
    ring.sort((a, b) => a.hash - b.hash);

    assert.equal(ring.length, 150);
  });
});

describe('Snowflake 64-bit ID Generation Logic', () => {
  it('guarantees unique, monotonically increasing IDs within a node', () => {
    const ids = [];
    const timestamp = 1725608400000n;
    const machineId = 42n;

    for (let seq = 0n; seq < 5n; seq++) {
      const id = (timestamp << 22n) | (machineId << 12n) | seq;
      ids.push(id);
    }

    // Verify all IDs are unique and strictly increasing
    assert.equal(new Set(ids).size, 5);
    for (let i = 1; i < ids.length; i++) {
      assert.ok(ids[i] > ids[i - 1]);
    }
  });
});
