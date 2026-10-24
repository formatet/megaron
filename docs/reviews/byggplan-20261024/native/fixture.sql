UPDATE players SET wanax_name='Leandros' WHERE id='bb6c8916-735b-42f8-b7a4-d34e4550d75a'

UPDATE players SET wanax_name='Myrine' WHERE id='bcd12b7f-347f-4be7-85b1-31e2cea5aee3'

UPDATE players SET wanax_name='Dorian' WHERE id='a3217b58-3227-4a59-8b23-59ea795c88d0'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='795eac3f-3fe4-4082-b592-0d69d4cdaddc' AND r=30 AND q BETWEEN 1 AND 7; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='795eac3f-3fe4-4082-b592-0d69d4cdaddc' AND q=0 AND r=31;

UPDATE units SET q=1,r=30,status='positioned' WHERE id='aa0c03c6-73fb-4309-aaa0-54735c4398f9'; UPDATE founder_phase SET q=1,r=30 WHERE host_unit_id='aa0c03c6-73fb-4309-aaa0-54735c4398f9';
