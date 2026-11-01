CREATE TABLE "t_things" (
    "id" uuid NOT NULL,
    "kind" text DEFAULT '''); DROP TABLE t; --' NOT NULL,
    "we""ird" text,
    CONSTRAINT "t_things_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "t_things_""x""_key" UNIQUE ("we""ird"),
    CONSTRAINT "t_things_kind_check" CHECK ("kind" IN ('a''b', '$$', '*/', '`', 'x
y', '''); DROP TABLE t; --', E'\\', '$erpora$', '"quoted"', '--', '/*', 'a;b'))
);
