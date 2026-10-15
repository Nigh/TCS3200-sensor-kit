"""Execute the actual PIO source with synthetic edges; no third-party emulator."""
from pathlib import Path

instructions, labels = [], {}
for raw in (Path(__file__).resolve().parents[1] / "src/period.pio").read_text().splitlines():
    line = raw.split(";", 1)[0].strip()
    if not line or line.startswith("."):
        continue
    if line.endswith(":"):
        labels[line[:-1]] = len(instructions)
    else:
        instructions.append(line.replace(",", "").split())
assert len(instructions) <= 32


def measure(pin, budget=30000):
    pc = x = isr = 0
    for cycle in range(budget):
        op, *args = instructions[pc]
        next_pc = pc + 1
        # The default two-cycle input synchronizer remains enabled.
        level = pin(cycle - 2)
        if op == "wait":
            if level != int(args[0]):
                next_pc = pc
        elif op == "mov":
            if args == ["x", "~null"]:
                x = 0xFFFFFFFF
            elif args == ["isr", "~x"]:
                isr = (~x) & 0xFFFFFFFF
            else:
                raise AssertionError(args)
        elif op == "jmp":
            if len(args) == 1:
                next_pc = labels[args[0]]
            elif args[0] == "pin":
                if level:
                    next_pc = labels[args[1]]
            elif args[0] == "x--":
                previous = x
                x = (x - 1) & 0xFFFFFFFF
                if previous:
                    next_pc = labels[args[1]]
            else:
                raise AssertionError(args)
        elif op == "push":
            return isr, cycle
        else:
            raise AssertionError(op)
        pc = next_pc
    return None


for period in (208, 250, 1250, 2500, 8333):
    for phase in range(0, period, max(1, period // 17)):
        result = measure(lambda t: int((t + phase) % period < period // 2))
        assert result is not None, (period, phase)
        counts, elapsed = result
        assert abs((3 * counts - 1) - period) <= 3, (period, phase, counts)
        assert elapsed < 30000
for duty in (0.1, 0.3, 0.7, 0.9):
    counts, _ = measure(lambda t: int(t % 1250 < int(1250 * duty)))
    assert abs((3 * counts - 1) - 1250) <= 3
assert measure(lambda t: 0) is None
assert measure(lambda t: 1) is None
assert measure(lambda t: int(t % 125000 < 62500)) is None
print("PIO full-period counting, phase/duty variation and no-edge deadlines passed")

# Model the longest documented filter response (one new period + 1us),
# arbitrary new phase/duty, and the exact dynamic channel cutoff.
clock = 125_000_000
for low in (4000, 5000, 10000, 15000, 19047, 19048, 600000):
    slot = max(250, (4_000_000 + low - 1) // low + 40)
    budget = (slot - 10 - 1) * (clock // 1_000_000)
    period = (clock + low - 1) // low
    settle = period + clock // 1_000_000
    for duty in (0.1, 0.5, 0.9):
        for phase in range(0, period, max(1, period // 9)):
            result = measure(lambda t: 1 if t < settle else int((t - settle + phase) % period < int(period * duty)), budget)
            assert result is not None, (low, duty, phase, slot)
            counts, elapsed = result
            assert abs((3 * counts - 1) - period) <= 3
            assert elapsed < budget
    assert measure(lambda t: 0, budget) is None
    assert measure(lambda t: 1, budget) is None
print("adaptive 240–1000Hz budgets include filter response, phase and duty margins")
