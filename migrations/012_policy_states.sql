CREATE TABLE IF NOT EXISTS download_policy_states (
    state CHAR(2) PRIMARY KEY,
    CHECK (state IN (
        'AC', 'AL', 'AP', 'AM', 'BA', 'CE', 'DF', 'ES', 'GO', 'MA',
        'MT', 'MS', 'MG', 'PA', 'PB', 'PR', 'PE', 'PI', 'RJ', 'RN',
        'RS', 'RO', 'RR', 'SC', 'SP', 'SE', 'TO'
    ))
);

INSERT INTO download_policy_states (state)
SELECT state
FROM (
    VALUES
        ('AC'::char(2)), ('AL'::char(2)), ('AP'::char(2)), ('AM'::char(2)), ('BA'::char(2)),
        ('CE'::char(2)), ('DF'::char(2)), ('ES'::char(2)), ('GO'::char(2)), ('MA'::char(2)),
        ('MT'::char(2)), ('MS'::char(2)), ('MG'::char(2)), ('PA'::char(2)), ('PB'::char(2)),
        ('PR'::char(2)), ('PE'::char(2)), ('PI'::char(2)), ('RJ'::char(2)), ('RN'::char(2)),
        ('RS'::char(2)), ('RO'::char(2)), ('RR'::char(2)), ('SC'::char(2)), ('SP'::char(2)),
        ('SE'::char(2)), ('TO'::char(2))
) AS states(state)
ON CONFLICT (state) DO NOTHING;
