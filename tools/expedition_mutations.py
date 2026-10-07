#!/usr/bin/env python3
"""Prove expedition invariants with real DB mutations; restore each source edit."""
import argparse
from pathlib import Path
import re
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output-dir', type=Path,
                        default=Path(tempfile.gettempdir()) / 'megaron-expedition-mutations')
    args = parser.parse_args()
    args.output_dir.mkdir(parents=True, exist_ok=True)
    root = Path(__file__).resolve().parents[1]
    source = root / 'server/internal/combat/expedition.go'
    original = source.read_text()

    def test(name, pattern, expect_failure=False):
        result = subprocess.run(
            ['tools/gotest.sh', './internal/combat/', '-run', pattern],
            cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        log = args.output_dir / (name + '.log')
        log.write_text(result.stdout)
        failures = re.findall(r'--- FAIL: (\S+)', result.stdout)
        if expect_failure:
            assert result.returncode and failures and 'build failed' not in result.stdout, (
                f'{name}: expected a named failing assertion, see {log}')
            assert any(re.search(pattern, failure) for failure in failures), (
                f'{name}: unrelated test failed, see {log}')
            print(f'{name}: RED ({", ".join(failures)})', flush=True)
        else:
            assert result.returncode == 0, f'{name}: baseline failed, see {log}'
            print(f'{name}: GREEN', flush=True)

    test('baseline', 'TestExpedition')
    start = original.index('\tcase currentTick+legTravelTicks')
    end = original.index('\n\t}', start)
    mutations = [
        ('half-time', original[:start] + original[end:],
         'TestExpeditionLifecycle_HalfTimeWhileRoamingNearHome'),
        ('path-sight', original.replace('walked = path',
         '_ = path; walked = []province.MapPosition{here}'),
         'TestExpedition_RecordsSightAlongTheWholeLeg'),
    ]
    start = original.index('h.eventStore.AppendTx',
                           original.index('func (h *UnitArrivalHandler) expeditionHome('))
    mutations.append(('atomic-report', original[:start] + original[start:].replace(
        'h.eventStore.AppendTx(ctx, tx,', 'h.eventStore.Append(ctx,', 1),
        'TestExpeditionLifecycle_AtomicReportRetry'))
    start = original.index('\n\tif reason == "" {\n\t\t_, returnCost')
    end = original.index('\n\tif reason == "" {\n\t\t_, arriveTick', start)
    mutations.append(('return-reservation', original[:start] + original[end:],
                      'TestExpeditionLifecycle_ReservesAsymmetricHomeRoute'))
    for name, mutated, pattern in mutations:
        assert mutated != original, f'{name}: mutation did not change source'
        try:
            source.write_text(mutated)
            subprocess.run(['gofmt', '-w', str(source)], check=True)
            test(name, pattern, expect_failure=True)
        finally:
            source.write_text(original)
    test('restored', 'TestExpedition')
    print(f'Proof logs: {args.output_dir}')


if __name__ == '__main__':
    main()
