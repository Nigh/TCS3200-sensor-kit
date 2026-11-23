#!/usr/bin/env python3
"""Capture and analyze TCS3200 electrical step-response traces on Linux."""
import argparse
import bisect
import csv
import fcntl
import glob
import json
import math
import os
import select
import statistics
import struct
import sys
import termios
import time

N = 1_000_000
TRIGGER = 500_000
SIZE = N // 8
RATES = {'R': 5_000_000, 'F': 5_000_000, 'r': 250_000, 'f': 250_000}
FIELDS = ('trace', 'profile', 'direction', 'dark_hz', 'light_hz', 'observe_us',
          'slope_window_us', 't10_us', 't90_us', 'transition_us',
          'peak_slope_hz_s', 'peak_slope_span_us', 'status')


def parse_frame(data, expected=None):
    header, sep, payload = data.partition(b'\n')
    if not sep or len(header) > 100 or len(payload) != SIZE:
        raise ValueError('invalid or truncated frame')
    try:
        parts = header.decode('ascii').split()
        if len(parts) != 5 or parts[0] != 'STEP1':
            raise ValueError()
        cmd = parts[1]
        if cmd not in RATES or (expected is not None and cmd != expected):
            raise ValueError()
        if tuple(map(int, parts[2:])) != (RATES[cmd], N, TRIGGER):
            raise ValueError()
    except (UnicodeError, ValueError):
        raise ValueError('invalid frame header') from None
    return cmd, payload


def periods(payload, rate):
    """Return (right-edge time, period, frequency, left index, right index)."""
    edges = []
    previous = payload[0] & 1
    for i, byte in enumerate(payload):
        # Transitions at bit zero depend on the preceding byte.
        starts = byte & ~((byte << 1) | previous) & 255
        while starts:
            bit = (starts & -starts).bit_length() - 1
            edges.append(i * 8 + bit)
            starts &= starts - 1
        previous = byte >> 7
    return [((b - TRIGGER) * 1e6 / rate, (b - a) * 1e6 / rate,
             rate / (b - a), a, b) for a, b in zip(edges, edges[1:])]


def plateau(points, start, end):
    return [p[2] for p in points if start <= p[3] and p[4] < end]


def median_mad(values):
    if len(values) < 3:
        return None
    med = statistics.median(values)
    return med, statistics.median(abs(v - med) for v in values)


def platforms(frames, lower=False):
    dark, light = [], []
    for cmd, points in frames.items():
        if cmd.islower() != lower:
            continue
        before = plateau(points, 0, 400_000)
        after = plateau(points, 600_000, N)
        if cmd.lower() == 'r':
            dark.extend(before)
            light.extend(after)
        else:
            light.extend(before)
            dark.extend(after)
    return median_mad(dark), median_mad(light)


def metrics(points, direction, settings, observe_us):
    dark, light, window = (settings[k] for k in ('dark_hz', 'light_hz', 'slope_window_us'))
    result = dict(t10_us=None, t90_us=None, transition_us=None,
                  peak_slope_hz_s=None, peak_slope_span_us=None)
    status = []
    bright = median_mad(plateau(points, 600_000, N) if direction == 'rise'
                        else plateau(points, 0, 400_000))
    if bright and abs(bright[0] - light) > light * 0.05:
        result['status'] = 'bright_baseline_mismatch'
        return result
    first = None
    second = None
    for p in points:
        t, _, f = p[:3]
        if not 0 < t <= observe_us:
            continue
        progress = ((f - dark) if direction == 'rise' else (light - f)) / (light - dark)
        if first is None and progress >= 0.1:
            first = t
        if first is not None and progress >= 0.9:
            second = t
            break
    result['t10_us'], result['t90_us'] = first, second
    if first is None or second is None:
        status.append('threshold_not_observed')
    elif first == second:
        status.append('transition_unresolved')
    else:
        result['transition_us'] = second - first
    if second is not None:
        times = [p[0] for p in points]
        gaps = [0]
        for i in range(1, len(points)):
            gaps.append(gaps[-1] + (times[i] - times[i - 1] > window / 2))
        best = None
        for i, p in enumerate(points):
            t1 = p[0]
            if t1 < -window:
                continue
            if t1 >= second:
                break
            j = bisect.bisect_left(times, t1 + window, i + 1)
            if j == len(points):
                continue
            t2 = times[j]
            span = t2 - t1
            if t2 <= 0 or t2 > observe_us or span > 1.25 * window or gaps[j] != gaps[i]:
                continue
            slope = (points[j][2] - p[2]) * 1e6 / span
            if (direction == 'rise' and slope > 0 and (best is None or slope > best[0]) or
                    direction == 'fall' and slope < 0 and (best is None or slope < best[0])):
                best = slope, span
        if best is not None:
            result['peak_slope_hz_s'], result['peak_slope_span_us'] = best
    if result['peak_slope_hz_s'] is None:
        status.append('slope_not_observed')
    result['status'] = ';'.join(status) or 'ok'
    return result


def preflight(frames, requested_window=None):
    fast_dark, fast_light = platforms(frames)
    if not fast_light or not 0 < fast_light[0] < 500_000:
        raise ValueError('fast bright platform missing or >= 500 kHz; check LED, wiring or reduce light')
    long_dark, long_light = platforms(frames, True)
    long_ok = bool(fast_light[0] <= 20_000 and long_light and
                   abs(long_light[0] - fast_light[0]) <= fast_light[0] * .05)
    if fast_light[0] <= 20_000 and not long_ok:
        print('long profile unavailable: bright platform missing or inconsistent', file=sys.stderr)
    dark = long_dark if long_ok and long_dark else fast_dark
    light = fast_light
    if not dark:
        raise ValueError('dark platform missing; check shielding and wiring or reduce LED light')
    if light[0] - dark[0] <= 6 * max(dark[1], light[1]) or light[0] <= dark[0]:
        raise ValueError('insufficient contrast; check shielding, wiring or LED distance')
    settings = dict(dark_hz=dark[0], light_hz=light[0], dark_mad_hz=dark[1],
                    light_mad_hz=light[1], slope_window_us=6000)
    selected = {}
    transitions = [2500]
    for direction, fast, slow in (('rise', 'R', 'r'), ('fall', 'F', 'f')):
        choices = [(fast, metrics(frames[fast], direction, settings, 100_000))]
        if long_ok and slow in frames:
            choices.append((slow, metrics(frames[slow], direction, settings, 2_000_000)))
        chosen = next((x for x in choices if x[1]['t90_us'] is not None), choices[-1])
        selected[direction] = (chosen, choices)
        if chosen[1]['transition_us'] is not None:
            transitions.append(chosen[1]['transition_us'])
    settings['slope_window_us'] = requested_window or max(6000, 1000 * math.ceil(1.5 * max(transitions) / 1000))
    for direction, (chosen, choices) in selected.items():
        t90 = chosen[1]['t90_us']
        required = max(25000, math.ceil(2 * t90 + 2 * settings['slope_window_us'])) if t90 is not None else None
        fast_t90 = choices[0][1]['t90_us']
        if fast_t90 is not None and required is not None and required <= 100_000:
            cmd = choices[0][0]
        else:
            cmd = choices[-1][0]
        limit = 100_000 if cmd.isupper() else 2_000_000
        if required is None or required > limit:
            print(f'{direction}: threshold missing or observation truncated to {limit} us', file=sys.stderr)
        settings[direction] = dict(command=cmd, observe_us=min(required, limit) if required is not None else limit)
    return settings


def read_frame(path):
    with open(path, 'rb') as f:
        return parse_frame(f.read())


def analyze(directory, include_periods=False):
    paths = sorted(glob.glob(os.path.join(directory, 'traces', '*.bin')))
    pilots = sorted(glob.glob(os.path.join(directory, 'pilot', '*.bin')))
    settings_path = os.path.join(directory, 'settings.json')
    settings = None
    if os.path.exists(settings_path):
        with open(settings_path) as f:
            settings = json.load(f)
    if not paths and not (include_periods and pilots):
        raise ValueError('no formal traces')
    if paths and settings is None:
        raise ValueError('settings.json missing')
    rows, period_rows = [], []
    for path in (pilots if include_periods else []) + paths:
        cmd, payload = read_frame(path)
        name = os.path.relpath(path, directory)
        points = periods(payload, RATES[cmd])
        if include_periods:
            period_rows.extend((name, *p[:3]) for p in points)
        if path in paths:
            direction = 'rise' if cmd.lower() == 'r' else 'fall'
            config = settings[direction]
            if cmd != config['command']:
                raise ValueError(f'{name}: profile differs from settings')
            row = dict(trace=name, profile='fast' if cmd.isupper() else 'long', direction=direction,
                       dark_hz=settings['dark_hz'], light_hz=settings['light_hz'],
                       observe_us=config['observe_us'], slope_window_us=settings['slope_window_us'])
            row.update(metrics(points, direction, settings, config['observe_us']))
            rows.append(row)
    if paths:
        with open(os.path.join(directory, 'summary.csv'), 'w', newline='') as f:
            writer = csv.DictWriter(f, fieldnames=FIELDS)
            writer.writeheader()
            writer.writerows(rows)
        for direction in ('rise', 'fall'):
            for profile in ('fast', 'long'):
                group = [r for r in rows if r['direction'] == direction and r['profile'] == profile]
                if not group:
                    continue
                print(f'{direction} {profile}: n={len(group)}')
                for key in ('t10_us', 't90_us', 'transition_us'):
                    values = sorted(r[key] for r in group if r[key] is not None)
                    if values:
                        print(f'  {key}: n={len(values)} median={statistics.median(values):.3f} p95={values[math.ceil(.95 * len(values))-1]:.3f}')
                    else:
                        print(f'  {key}: n=0')
                slopes = [r['peak_slope_hz_s'] for r in group if r['peak_slope_hz_s'] is not None]
                print(f'  peak_slope_hz_s: n={len(slopes)}' + (f' extreme={(max(slopes) if direction == "rise" else min(slopes)):.3f}' if slopes else ''))
    if include_periods:
        with open(os.path.join(directory, 'periods.csv'), 'w', newline='') as f:
            writer = csv.writer(f)
            writer.writerow(('trace', 't_us', 'period_us', 'frequency_hz'))
            writer.writerows(period_rows)
    return rows


def receive(fd, command):
    deadline = time.monotonic() + 10
    if os.write(fd, command.encode()) != 1:
        raise OSError('short command write')
    data = bytearray()
    header_end = None
    while header_end is None or len(data) < header_end + SIZE:
        remaining = deadline - time.monotonic()
        if remaining <= 0 or not select.select([fd], [], [], remaining)[0]:
            raise TimeoutError('frame timeout or short read')
        chunk = os.read(fd, 65536 if header_end is not None else 1)
        if not chunk:
            raise EOFError('short read')
        data.extend(chunk)
        if header_end is None and b'\n' in data:
            header_end = len(data)
            if data == b'ERR capture\n':
                raise ValueError('ERR capture')
            if len(data) > 100 or not data.startswith(b'STEP1 '):
                raise ValueError('unknown frame header')
        if header_end is None and len(data) > 100:
            raise ValueError('unknown frame header')
    parse_frame(data, command)
    return bytes(data)


def capture(port, directory, repeats, requested_window):
    os.mkdir(directory)
    os.mkdir(os.path.join(directory, 'pilot'))
    os.mkdir(os.path.join(directory, 'traces'))
    fd = os.open(port, os.O_RDWR | os.O_NOCTTY | os.O_NONBLOCK)
    try:
        attrs = termios.tcgetattr(fd)
        attrs[0] = attrs[1] = attrs[3] = 0
        attrs[2] = termios.CS8 | termios.CREAD | termios.CLOCAL
        attrs[4] = attrs[5] = termios.B115200
        attrs[6][termios.VMIN] = 0
        attrs[6][termios.VTIME] = 0
        termios.tcsetattr(fd, termios.TCSANOW, attrs)
        fcntl.ioctl(fd, termios.TIOCMBIS, struct.pack('I', termios.TIOCM_DTR))
        termios.tcflush(fd, termios.TCIOFLUSH)
        frames = {}
        def acquire(cmd, folder, number):
            data = receive(fd, cmd)
            with open(os.path.join(directory, folder, f'{number:03d}-{cmd}.bin'), 'xb') as f:
                f.write(data)
            _, payload = parse_frame(data, cmd)
            return periods(payload, RATES[cmd])
        for i, cmd in enumerate('RF', 1):
            frames[cmd] = acquire(cmd, 'pilot', i)
        _, bright = platforms(frames)
        if bright and 0 < bright[0] <= 20_000:
            for i, cmd in enumerate('rf', 3):
                frames[cmd] = acquire(cmd, 'pilot', i)
        settings = preflight(frames, requested_window)
        print(json.dumps(settings, indent=2), flush=True)
        with open(os.path.join(directory, 'settings.json'), 'x') as f:
            json.dump(settings, f, indent=2)
            f.write('\n')
        for i in range(repeats):
            for j, direction in enumerate(('rise', 'fall'), 1):
                acquire(settings[direction]['command'], 'traces', 2 * i + j)
    finally:
        os.close(fd)
    analyze(directory)


def positive(value):
    n = int(value)
    if n <= 0:
        raise argparse.ArgumentTypeError('must be positive')
    return n


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='action', required=True)
    cap = sub.add_parser('capture')
    cap.add_argument('--port', required=True)
    cap.add_argument('--out', required=True)
    cap.add_argument('--repeats', type=positive, default=10)
    cap.add_argument('--slope-window-ms', type=int, choices=range(5, 501), metavar='5..500')
    ana = sub.add_parser('analyze')
    ana.add_argument('--data', required=True)
    ana.add_argument('--periods', action='store_true')
    args = parser.parse_args()
    try:
        if args.action == 'capture':
            capture(args.port, args.out, args.repeats,
                    args.slope_window_ms * 1000 if args.slope_window_ms else None)
        else:
            analyze(args.data, args.periods)
    except (OSError, ValueError, EOFError, KeyError, TypeError) as exc:
        print(f'error: {exc}', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
