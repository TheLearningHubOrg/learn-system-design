/**
 * Twitter Snowflake 64-bit Unique Distributed ID Generator
 *
 * Bit Layout:
 * 1 bit  : Unused sign bit (always 0)
 * 41 bits: Epoch timestamp in milliseconds (gives 69 years of IDs)
 * 10 bits: Machine / Node ID (supports up to 1024 unique instances)
 * 12 bits: Sequence counter (supports up to 4096 IDs per ms per node)
 */

export class SnowflakeIdGenerator {
  constructor(machineId = 1n, customEpoch = 1704067200000n) { // 2024-01-01T00:00:00Z
    if (machineId < 0n || machineId > 1023n) {
      throw new Error('machineId must be between 0 and 1023');
    }
    this.machineId = BigInt(machineId);
    this.epoch = BigInt(customEpoch);
    this.sequence = 0n;
    this.lastTimestamp = -1n;
  }

  nextId() {
    let timestamp = BigInt(Date.now());

    if (timestamp < this.lastTimestamp) {
      throw new Error(`Clock moved backwards! Refusing to generate ID for ${this.lastTimestamp - timestamp} ms`);
    }

    if (timestamp === this.lastTimestamp) {
      this.sequence = (this.sequence + 1n) & 4095n;
      if (this.sequence === 0n) {
        // Sequence rollover in same ms: busy wait until next ms
        while (timestamp <= this.lastTimestamp) {
          timestamp = BigInt(Date.now());
        }
      }
    } else {
      this.sequence = 0n;
    }

    this.lastTimestamp = timestamp;

    const timePart = (timestamp - this.epoch) << 22n;
    const nodePart = this.machineId << 12n;
    const id = timePart | nodePart | this.sequence;

    return id.toString();
  }
}
