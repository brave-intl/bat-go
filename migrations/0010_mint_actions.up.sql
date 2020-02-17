create table mint_actions (
  id uuid PRIMARY KEY NOT NULL default uuid_generate_v4(),
  created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT current_timestamp,
  "value" TEXT NOT NULL,
  action_id TEXT NOT NULL,
  modal_id TEXT NOT NULL
);

create index modal_action_key on mint_actions(action_id, modal_id);
