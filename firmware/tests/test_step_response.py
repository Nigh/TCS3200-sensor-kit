import importlib.util
import os
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('step_response', Path(__file__).parents[1] / 'tools' / 'step_response.py')
s = importlib.util.module_from_spec(spec)
spec.loader.exec_module(s)


def waveform(cmd, shaped=False, delayed=False):
    rate = s.RATES[cmd]
    half_us = 100_000 if cmd.isupper() else 2_000_000
    rise = cmd.lower() == 'r'
    edges = list(range(-half_us, 1, 1000 if rise else 200))
    if shaped and rise:
        edges += list(range(500, 6501, 500)) + list(range(6700, half_us, 200))
    elif delayed and not rise:
        edges += list(range(200, min(150_000, half_us), 200))
        edges += list(range(150_000, half_us, 1000))
    else:
        edges += list(range(200 if rise else 1000, half_us, 200 if rise else 1000))
    data = bytearray(s.SIZE)
    for t in edges:
        i = s.TRIGGER + t * rate // 1_000_000
        if 0 <= i < s.N:
            data[i // 8] |= 1 << (i % 8)
    return bytes(data)


def points(cmd, **kwargs):
    return s.periods(waveform(cmd, **kwargs), s.RATES[cmd])


class StepResponseTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.frames = {c: points(c, shaped=True) for c in 'RFrf'}

    def test_preflight_shaped_transition(self):
        settings = s.preflight(self.frames)
        self.assertEqual((settings['dark_hz'], settings['light_hz']), (1000, 5000))
        self.assertEqual(settings['slope_window_us'], 10000)
        self.assertEqual(settings['rise'], {'command': 'R', 'observe_us': 33400})
        up = s.metrics(self.frames['R'], 'rise', settings, 100000)
        self.assertEqual((up['t10_us'], up['t90_us'], up['transition_us']), (500, 6700, 6200))
        self.assertGreater(up['peak_slope_hz_s'], 0)
        self.assertTrue(10000 <= up['peak_slope_span_us'] <= 12500)
        down = s.metrics(self.frames['F'], 'fall', settings, 100000)
        self.assertIsNone(down['transition_us'])
        self.assertIn('transition_unresolved', down['status'])

    def test_direct_slope_and_gap(self):
        settings = s.preflight(self.frames, 6000)
        for cmd, direction, expected in [('R', 'rise', 4000000 / 6), ('F', 'fall', -4000000 / 6)]:
            p = points(cmd)
            result = s.metrics(p, direction, settings, 100000)
            self.assertAlmostEqual(result['peak_slope_hz_s'], expected)
            self.assertEqual(result['peak_slope_span_us'], 6000)
            # Remove frequency points around t=0 so every crossing window has a 4 ms gap.
            p = [x for x in p if not -2000 < x[0] < 2000]
            result = s.metrics(p, direction, settings, 100000)
            self.assertIsNotNone(result['t90_us'])
            self.assertIsNone(result['peak_slope_hz_s'])
            self.assertIn('slope_not_observed', result['status'])

    def test_long_fall(self):
        frames = dict(self.frames)
        frames['F'] = points('F', delayed=True)
        frames['f'] = points('f', delayed=True)
        settings = s.preflight(frames)
        self.assertEqual(settings['fall']['command'], 'f')
        self.assertGreater(settings['fall']['observe_us'], 150000)

    def test_invalid_platforms_and_frames(self):
        for b in (bytes(s.SIZE), bytes([255]) * s.SIZE):
            empty = s.periods(b, s.RATES['R'])
            with self.assertRaises(ValueError):
                s.preflight({'R': empty, 'F': empty})
        with self.assertRaises(ValueError):
            s.preflight({'R': points('R'), 'F': points('R')})
        with self.assertRaises(ValueError):
            s.parse_frame(b'STEP1 R 5000000 1000000 500000\n' + bytes(s.SIZE - 1))

    def test_pilot_only_period_export(self):
        with tempfile.TemporaryDirectory() as directory:
            os.mkdir(os.path.join(directory, 'pilot'))
            with open(os.path.join(directory, 'pilot', '001-R.bin'), 'wb') as f:
                f.write(b'STEP1 R 5000000 1000000 500000\n' + waveform('R'))
            s.analyze(directory, True)
            with open(os.path.join(directory, 'periods.csv')) as f:
                self.assertIn('pilot/001-R.bin', f.read())
            with self.assertRaises(ValueError):
                s.analyze(directory)


if __name__ == '__main__':
    unittest.main()
