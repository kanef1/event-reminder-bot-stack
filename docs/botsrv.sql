CREATE TABLE "events" (
                          "eventId" SERIAL NOT NULL,
                          "userTgId" int4 NOT NULL,
                          "message" text NOT NULL,
                          "sendAt" timestamp with time zone NOT NULL,
                          "createdAt" timestamp with time zone NOT NULL DEFAULT now(),
                          "statusId" int4 NOT NULL DEFAULT 1,
                          "weekdays" integer[],
                          "periodicity" varchar(16) CHECK (periodicity IN ('hour', 'day', 'week', 'weekdays', NULL)),
                          PRIMARY KEY("eventId")
);