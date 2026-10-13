"""SilverAuditV2 reader tests; optional real, strictly read-only proof.

 python3 tools/alfa_economy_test.py
 ALFA_AUDIT_JSON=/path/to/actual-server-payload.json python3 tools/alfa_economy_test.py
 ALFA_TEST_DSN=postgres://... python3 tools/alfa_economy_test.py
 ALFA_TEST_PSQL_CMD='docker exec -i rig-pg psql -U poleia -d poleia' python3 tools/alfa_economy_test.py
 ALFA_TEST_WORLD=<uuid> optionally selects the audited world.
"""
import json
import math
import os
import unittest
from pathlib import Path

import alfa_db
import alfa_economy as E


def payload():
    # Matches SilverAuditV2Payload. Includes all seven stock buckets; unowned
    # is a subset, not a further bucket. Populations may include zero holdings.
    return dict(mining_epoch="a1111111-1111-4111-8111-111111111111", audit_tick=20, previous_audit_tick=19, city_silver=100,
                buy_escrow_silver=20, sell_escrow_silver=3, founder_silver=60,
                carried_silver=30, transport_silver=10, deferred_trade_silver=7,
                unowned_silver=10, total_silver=230, mining_credited_total=12,
                mined_since_last=2, audit_prev_total=228, net_delta=2,
                wanax_distribution=dict(population='world records and asset owners, including AI and dispossessed Wanax',
                                        count=4, total_silver=220, median_silver=10,
                                        max_silver=200, zero_count=2, zero_share=0.5),
                wanax_totals=[dict(owner_id='a', total_silver=0), dict(owner_id='b', total_silver=0),
                             dict(owner_id='c', total_silver=20), dict(owner_id='d', total_silver=200)])


class SilverAuditReaderTests(unittest.TestCase):
    def test_complete_total_distribution_and_realized_mining(self):
        p = payload()
        text = E.render_silver_audits([dict(world_tick=20, payload=p)], [])
        self.assertIn('total_silver', text)
        self.assertIn('230.00', text)
        self.assertIn(p['mining_epoch'], text)
        self.assertIn('BUY-escrow', text)
        self.assertIn('SELL-escrow', text)
        self.assertIn('väntande handelsretur', text)
        self.assertIn('19→20', text)
        self.assertIn('12.00', text)
        self.assertIn('audit_prev_total', text)
        self.assertIn('228.00', text)
        self.assertIn('net_delta', text)
        self.assertIn('Övrigt netto = net_delta − mined_since_last: 0.00 silver', text)
        self.assertIn('kranar minus sänkor', text)
        self.assertIn('inte automatiskt ett bokföringsfel', text)
        self.assertIn('2.00', text)
        self.assertIn('median_silver', text)
        self.assertIn('zero_share', text)
        self.assertIn('0.5000', text)
        self.assertIn(p['wanax_distribution']['population'], text)
        self.assertNotIn('liquid_total', text)
        self.assertNotIn('240.00', text, 'unowned subset must not inflate world total')
        p['audit_prev_total'], p['net_delta'] = 225, 5
        changed = E.render_silver_audits([dict(payload=p)], [])
        self.assertIn('225.00', changed)
        self.assertIn('Övrigt netto = net_delta − mined_since_last: 3.00 silver', changed)

    def test_v2_query_never_reads_v1(self):
        calls = []
        expected = [dict(payload=payload())]
        def query(sql):
            calls.append(sql)
            self.assertIn("event_type = 'SilverAuditV2'", sql)
            return expected
        self.assertEqual(E.load_silver_audits(query), (expected, []))
        self.assertEqual(len(calls), 1)

    def test_legacy_fallback_is_explicit_and_not_a_complete_total(self):
        calls = []
        historical = [dict(world_tick=5, payload=dict(liquid_total=99, fund_total=17, escrow_total=11))]
        def query(sql):
            calls.append(sql)
            return [] if 'SilverAuditV2' in sql else historical
        v2, v1 = E.load_silver_audits(query)
        self.assertEqual(len(calls), 2)
        self.assertIn("event_type = 'SilverAudit'", calls[1])
        text = E.render_silver_audits(v2, v1)
        self.assertIn('Historisk V1', text)
        self.assertIn('inte komplett världstotal', text)
        self.assertIn('99.00', text)
        self.assertNotIn('127.00', text)
        self.assertNotIn('median_silver', text)

    def test_first_v2_has_its_own_baseline_and_ignores_supplied_v1(self):
        p = payload()
        p['previous_audit_tick'] = None
        p['mined_since_last'] = 0
        text = E.render_silver_audits([dict(payload=p)], [dict(world_tick=1, payload=dict(liquid_total=999999))])
        self.assertIn('öppningsrevision (egen baslinje)', text)
        self.assertNotIn('999999', text)
        self.assertNotIn('Historisk V1', text)

    def test_no_audits_is_explicit(self):
        self.assertIn('inga audit-event', E.render_silver_audits([], []))

    def check_actual_payload(self, p):
        text = E.render_silver_audits([dict(payload=p)], [])
        self.assertIn('Komplett silverrevision', text)
        self.assertTrue(math.isclose(p['total_silver'], sum(p[key] for key, _ in E.SILVER_BUCKETS), abs_tol=1e-7))
        d = p['wanax_distribution']
        self.assertTrue(math.isclose(d['total_silver'] + p['unowned_silver'], p['total_silver'], abs_tol=1e-7))
        self.assertTrue(math.isclose(sum(w['total_silver'] for w in p['wanax_totals']), d['total_silver'], abs_tol=1e-7))
        self.assertEqual(len(p['wanax_totals']), d['count'])
        self.assertGreaterEqual(d['zero_share'], 0)
        self.assertLessEqual(d['zero_share'], 1)
        expected_share = d['zero_count'] / d['count'] if d['count'] else 0
        self.assertTrue(math.isclose(expected_share, d['zero_share'], abs_tol=1e-7))
        self.assertGreaterEqual(p['mined_since_last'], 0)
        self.assertGreaterEqual(p['mining_credited_total'], p['mined_since_last'])

    @unittest.skipUnless(os.environ.get('ALFA_AUDIT_JSON'), 'set ALFA_AUDIT_JSON for actual server JSON')
    def test_actual_json(self):
        value = json.loads(Path(os.environ['ALFA_AUDIT_JSON']).read_text())
        self.check_actual_payload(value.get('payload', value))

    @unittest.skipUnless(os.environ.get('ALFA_TEST_DSN') or os.environ.get('ALFA_TEST_PSQL_CMD'), 'set read-only test DB connection')
    def test_actual_db_event(self):
        db = alfa_db.Db(dsn=os.environ.get('ALFA_TEST_DSN'), psql_cmd=os.environ.get('ALFA_TEST_PSQL_CMD'))
        world = os.environ.get('ALFA_TEST_WORLD')
        if not world:
            found = db.rows("SELECT world_id FROM events WHERE event_type='SilverAuditV2' ORDER BY id DESC LIMIT 1")
            self.assertTrue(found, 'configured DB must contain an actual SilverAuditV2 event')
            world = found[0]['world_id']
        query = lambda sql: db.rows(sql.replace('{W}', world.replace("'", '')))
        audits, historical = E.load_silver_audits(query)
        self.assertTrue(audits, 'selected world must contain SilverAuditV2')
        self.assertFalse(historical)
        for audit in audits:
            self.check_actual_payload(audit['payload'])


if __name__ == '__main__':
    unittest.main()
