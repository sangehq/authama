-- Operations can be about a token, named "<tool>/<host>", for `history --token`.
-- SQLite cannot widen a CHECK constraint in place, so the table is rebuilt with
-- every row it had; nothing references subjects, so nothing else moves.
create table subjects_v2(
  event_id integer references events(id),
  op_id text references ops(op_id),
  kind text not null check (kind in ('profile','host','key','target','token')),
  name text not null
);
insert into subjects_v2(event_id, op_id, kind, name) select event_id, op_id, kind, name from subjects;
drop table subjects;
alter table subjects_v2 rename to subjects;
create index subjects_kind_name on subjects(kind, name);
