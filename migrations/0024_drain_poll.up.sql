--- drain poll table holds the identifier handed out by
--- /v2/suggestions/claim which will track the progress of
--- the various drain jobs.  Of all the tokens passed in
--- they are segregated by promotion id, and turned into
--- drain jobs.
create table drain_poll (
    id uuid primary key not null default uuid_generate_v4(),
    status varchar(32) not null default 'pending'
);

--- claim drain poll table is the linkage between
--- the claim_drain table and the drain_poll table
create table claim_drain_poll (
    drain_poll_id uuid not null,
    claim_drain_id uuid not null,
    complete boolean not null default false,
    primary key (drain_poll_id, claim_drain_id)
);
