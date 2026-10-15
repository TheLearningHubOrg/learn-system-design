/**
 * Consistent Hashing Ring with Virtual Nodes
 */

export function fnv1aHash(str) {
  let h = 0x811c9dc5;
  for (let i = 0; i < str.length; i++) {
    h ^= str.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return h >>> 0; // Unsigned 32-bit int
}

export class ConsistentHashRing {
  constructor(virtualNodes = 50) {
    this.virtualNodes = virtualNodes;
    this.ring = []; // Array of { hash: number, node: string }
    this.physicalNodes = new Set();
  }

  addNode(node) {
    if (this.physicalNodes.has(node)) return;
    this.physicalNodes.add(node);

    for (let i = 0; i < this.virtualNodes; i++) {
      const vNodeKey = `${node}#VN${i}`;
      const hashVal = fnv1aHash(vNodeKey);
      this.ring.push({ hash: hashVal, node });
    }

    this.ring.sort((a, b) => a.hash - b.hash);
  }

  removeNode(node) {
    if (!this.physicalNodes.has(node)) return;
    this.physicalNodes.delete(node);
    this.ring = this.ring.filter(item => item.node !== node);
  }

  getNode(key) {
    if (this.ring.length === 0) return null;

    const keyHash = fnv1aHash(key);

    // Binary search clockwise on ring
    let low = 0;
    let high = this.ring.length - 1;

    while (low <= high) {
      const mid = Math.floor((low + high) / 2);
      if (this.ring[mid].hash >= keyHash) {
        high = mid - 1;
      } else {
        low = mid + 1;
      }
    }

    // Wrap around to 0 if key exceeds highest hash on ring
    const index = low === this.ring.length ? 0 : low;
    return this.ring[index].node;
  }
}
