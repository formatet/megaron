import tempfile
import unittest
from pathlib import Path
import verb_parity as p

class ParserTests(unittest.TestCase):
    def test_nested_chi_and_group_and_helper(self):
        source = '''
        // r.Post("/api/v1/ghost", ghost)
        r.Route("/api/v1", func(r chi.Router) {
          r.Group(func(r chi.Router) {
            r.With(auth()).Get("/worlds/{worldID}/units", list)
          })
          r.Route("/worlds/{worldID}/kingdoms", func(r chi.Router) {
            r.Patch("/{kingdomID}/council/{role}", assign)
          })
          r.Post("/worlds/{worldID}/join", join)
        })
        func registerReturnArmyRoute(r chi.Router, h http.HandlerFunc) {
          r.With(gate).Post("/worlds/{worldID}/settlements/{id}/return-army", h)
        }
        '''
        rows = p.route_inventory(source, {'registerReturnArmyRoute': '/api/v1'})
        self.assertEqual([(x['method'],x['path']) for x in rows], [
            ('GET','/api/v1/worlds/{id}/units'),
            ('PATCH','/api/v1/worlds/{id}/kingdoms/{id}/council/{id}'),
            ('POST','/api/v1/worlds/{id}/join'),
            ('POST','/api/v1/worlds/{id}/settlements/{id}/return-army')])

    def test_go_format_and_delete_item(self):
        rows, unknown = p.calls('''func cmd() {
          path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/placements", world, city)
          c.post(path, body)
          c.delete(fmt.Sprintf("%s/%d", path, ordinal))
          // c.get("/api/v1/not-real")
        }''','go')
        self.assertEqual([(r['method'],r['path']) for r in rows],[
            ('POST','/api/v1/worlds/{id}/provinces/{id}/placements'),
            ('DELETE','/api/v1/worlds/{id}/provinces/{id}/placements/{id}')])
        self.assertFalse(unknown)

    def test_factory_parameter_binding(self):
        rows, unknown = p.calls('''func routeCmd(action string) {
          c.post(fmt.Sprintf("/api/v1/worlds/%s/standing-orders/%s/%s", world, id, action), nil)
        }''', 'go', bindings={'routeCmd': {'action': {'pause','resume'}}})
        self.assertEqual({x['path'] for x in rows}, {
            '/api/v1/worlds/{id}/standing-orders/{id}/pause',
            '/api/v1/worlds/{id}/standing-orders/{id}/resume'})

    def test_js_templates_concatenation_and_method_shorthand(self):
        rows, _ = p.calls('''
          const method = checked ? 'PUT' : 'DELETE';
          fetchAuth(`/api/v1/notification-preferences/${encodeURIComponent(kind)}`, { method });
          fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/units/' + id + '/march', {method:'POST'});
        ''','js')
        self.assertEqual({(x['method'],x['path']) for x in rows}, {
            ('PUT','/api/v1/notification-preferences/{id}'),
            ('DELETE','/api/v1/notification-preferences/{id}'),
            ('POST','/api/v1/worlds/{id}/units/{id}/march')})

    def test_regex_quotes_do_not_eat_later_calls(self):
        rows, _ = p.calls('''const clean = html.replace(/[&<>"']/g, escape);
        fetchAuth('/api/v1/units'); const ratio = total / count;
        fetchAuth('/api/v1/goods');''','js')
        self.assertEqual([r['path'] for r in rows],['/api/v1/units','/api/v1/goods'])

    def test_multiline_ternary_and_query(self):
        rows, _ = p.calls('''const base = `/api/v1/worlds/${world}/notifications`;
        const url = kind
          ? `${base}?kind=${kind}`
          : `${base}?exclude=${exclude}`;
        fetchAuth(url);''','js')
        self.assertEqual({r['path'] for r in rows},{'/api/v1/worlds/{id}/notifications'})

    def test_injected_object_property(self):
        rows, _ = p.calls('''function outcome(ctx) {
        const {placementsURL, fetchImpl} = ctx;
        fetchImpl(placementsURL, {method:'POST'});
        fetchImpl(`${placementsURL}/${id}`, {method:'DELETE'});
        }
        outcome({placementsURL: `/api/v1/worlds/${world}/provinces/${city}/placements`,
           fetchImpl: fetchAuth});''','js')
        self.assertEqual({(r['method'],r['path']) for r in rows},{
            ('POST','/api/v1/worlds/{id}/provinces/{id}/placements'),
            ('DELETE','/api/v1/worlds/{id}/provinces/{id}/placements/{id}')})

    def test_unresolved_method_never_becomes_get(self):
        rows, unknown = p.calls("fetchAuth('/api/v1/units', {method: dynamic});",'js')
        self.assertFalse(rows); self.assertEqual(len(unknown),1)

    def test_block_shadow_does_not_escape(self):
        rows, _ = p.calls('''let path = '/api/v1/units';
        { let path = '/api/v1/goods'; fetchAuth(path); }
        fetchAuth(path);''','js')
        self.assertEqual([r['path'] for r in rows],['/api/v1/goods','/api/v1/units'])

    def test_allowlist_and_bounded_article_alias(self):
        route = {'method':'POST','path':'/api/v1/units'}
        self.assertEqual(p.exclusions(route,[{'route':'* /api/v1/units','reason':'fixture'}]),['fixture'])
        docs = p.documentation([route],{'units.md':'Train a unit','other.md':'Train'},
            {'/api/v1/units':{'terms':['Train'],'articles':['units.md']}})
        self.assertEqual(docs['POST /api/v1/units'],['units.md:1'])

    def test_verblista_methods_optional_and_world_prefix(self):
        specs = p.verblist_routes('`GET/PUT /worlds/:id/retreat-default` `POST /units/:id/march` `PUT/DELETE /notification-preferences[/:kind]`')
        self.assertIn('GET /api/v1/worlds/{id}/retreat-default',specs)
        self.assertIn('POST /api/v1/worlds/{id}/units/{id}/march',specs)
        self.assertIn('PUT /api/v1/notification-preferences/{id}',specs)

    def test_verblista_leaf_paths_inherit_context(self):
        text = "| load / unload | `POST /units/:id/load`, `/unload` | CLI | web |\n| route | `POST/DELETE /standing-orders[/:id]`, `/pause`, `/resume` | CLI | web |"
        specs = p.verblist_routes(text)
        self.assertIn('POST /api/v1/worlds/{id}/units/{id}/unload', specs)
        self.assertIn('POST /api/v1/worlds/{id}/standing-orders/{id}/pause', specs)
        self.assertIn('DELETE /api/v1/worlds/{id}/standing-orders/{id}', specs)
        self.assertNotIn('POST /api/v1/worlds/{id}/standing-orders/{id}', specs)

    def test_full_inventory_and_cross_file_helper(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            files={
                'server/cmd/server/main.go':'r.Route("/api/v1", func(r chi.Router) { r.Get("/worlds/{id}/preview", h) })',
                'server/cmd/keryx/helper.go':'func show(c *Client, path string) { c.get(path) }',
                'server/cmd/keryx/main.go':'func cmd() { show(c, fmt.Sprintf("/api/v1/worlds/%s/preview", world)) }',
                'web/static/js/megaron/main.js':"fetchAuth(`/api/v1/worlds/${world}/preview`);",
                'web/static/codex/test.md':'Preview the city',
            }
            for file,text in files.items():
                target=root/file;target.parent.mkdir(parents=True,exist_ok=True);target.write_text(text)
            config={'helper_prefixes':{},'allowlist':[], 'codex_aliases':{'/api/v1/worlds/{id}/preview':{'terms':['Preview'],'articles':['test.md']}}}
            result=p.inventory(root,config,'`GET /worlds/:id/preview`')
            route=result['routes'][0]
            self.assertTrue(all(route[x] for x in ('keryx','webb','codex')))
            self.assertFalse(result['listed_not_registered']);self.assertFalse(result['registered_not_listed'])

if __name__ == '__main__': unittest.main()
