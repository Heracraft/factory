---
title: Pricing
description: What a project costs running and stopped, and where to see what you've used.
section: Account
order: 30
---

You pay per project, for three things.

**Compute**, by the minute while the machine is running, up to a monthly cap. A machine left running all month costs the cap.

| Size    | vCPU | Memory | Per hour | Monthly cap |
| ------- | ---- | ------ | -------- | ----------- |
| `small` | 2    | 4 GB   | $0.07    | $49         |
| `large` | 4    | 8 GB   | $0.14    | $99         |
| `xl`    | 8    | 16 GB  | $0.28    | $199        |

**Disk**, $0.10 per GB per month on the disk's full size, running or stopped, until you destroy the project. A `large` project's 40 GB disk is $4 a month. Snapshots are free.

**Egress**, data the machine sends to the internet: 500 GB per project per month included, then $0.05 per GB. Incoming data and your own SSH traffic, port forwards included, don't count.

Your first day of compute is free.

## Examples

- `large`, running all month: $99 + $4 disk = $103.
- `large`, stopped 10 hours every night: about $58 + $4 = $62.
- `small`, used for a day and then left stopped: $1.68, then $2 a month for the disk until you destroy it.

## Seeing what you've used

`repose projects` and `repose status` show each project's cost today and this month. The dashboard adds the month projected at the current rate. Usage is totalled a few minutes past each hour, so figures can trail by up to an hour.

## Stopping the charges

- `repose stop` ends compute. Disk continues.
- `repose destroy` ends everything for that project. Its final snapshot is kept free for 30 days.
