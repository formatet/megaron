#!/usr/bin/env python3
"""Check FINAL fishing evidence; older-rule logs are deliberately ignored."""
from pathlib import Path
p=Path(__file__).resolve().parent
assert 'FAIL: TestFinalFishingNumbers' in (p/'final-rule-baseline-red.log').read_text()
red=(p/'final-mutation-red.log').read_text()
for name in ['TestBayLoadedOwnerCountsRiversButNotMissingMap','TestBayFishingHTTPUsesLocalCapacity']:
    assert 'FAIL: '+name in red
restored=(p/'final-mutation-restored-green.log').read_text()
assert 'FAIL' not in restored and sum(line.startswith('ok') for line in restored.splitlines())==2
assert '# fail 0' in (p/'final-js.log').read_text()
assert not (p/'final-vet.log').read_text().strip()
full=(p/'review-full-fresh.log').read_text()
assert 'FAIL' not in full and 'internal/world' in full
print('FINAL named RED, physical mutation RED/restore GREEN, JS/vet/full Go evidence verified')

review_red=(p/'review-mutation-red.log').read_text()
for name in ['TestFishingResumeRecomputesAfterGlobalTransition','TestFishingTransitionPrunesHighestAndRecomputesOnce']:
    assert 'FAIL: '+name in review_red
assert 'FAIL' not in (p/'review-mutation-restored-green.log').read_text()
assert not (p/'review-resume-vet.log').read_text().strip()
print('Activation/resume mutation RED/restore GREEN and fresh full review gates verified')
