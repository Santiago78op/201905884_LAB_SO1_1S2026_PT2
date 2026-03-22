-- Tabla X

CREATE TABLE X (
    A CHAR(1) NOT NULL,
    B CHAR(1) NOT NULL,
    C CHAR(1) NOT NULL,
    D CHAR(1) NOT NULL
);

-- Tabla Y

CREATE TABLE Y (
    A CHAR(1) NOT NULL,
    B CHAR(1) NOT NULL,
    C CHAR(1) NOT NULL,
    D CHAR(1) NOT NULL
);

-- Tabla Z

CREATE TABLE Z (
    C CHAR(1) NOT NULL,
    D CHAR(1) NOT NULL
);

-- Insertar datos en la tabla X
INSERT INTO X VALUES ('a','b','c','d');
INSERT INTO X VALUES ('a','b','e','f');
INSERT INTO X VALUES ('b','c','e','f');
INSERT INTO X VALUES ('e','d','c','d');
INSERT INTO X VALUES ('b','c','c','d');
INSERT INTO X VALUES ('a','b','d','e');

SELECT * FROM X;

-- Insertar datos en la tabla Y
INSERT INTO Y VALUES ('b','c','e','f');
INSERT INTO Y VALUES ('e','d','c','d');
INSERT INTO Y VALUES ('b','c','c','d');
INSERT INTO Y VALUES ('a','b','d','e');
INSERT INTO Y VALUES ('a','b','c','d');
INSERT INTO Y VALUES ('a','b','e','f');
INSERT INTO Y VALUES ('g','h','i','j');

SELECT * FROM Y;

-- Insertar datos en la tabla Z
INSERT INTO Z VALUES ('c','d');
INSERT INTO Z VALUES ('e','f');

SELECT * FROM Z;

-- Pregunta 1, Union de X y Y
SELECT DISTINCT valor
FROM (
    -- Primero juntamos todos los valores de todas las columnas de X e Y
    SELECT A AS valor FROM X UNION ALL SELECT B FROM X UNION ALL SELECT C FROM X UNION ALL SELECT D FROM X
    UNION ALL
    SELECT A FROM Y UNION ALL SELECT B FROM Y UNION ALL SELECT C FROM Y UNION ALL SELECT D FROM Y
)
ORDER BY valor;

-- Pregunta 2, Interseccion de X y Y
SELECT A FROM X
INTERSECT
SELECT A FROM Y;
    
SELECT DISTINCT valor
FROM (
    (SELECT A AS valor FROM X
     INTERSECT
     SELECT A FROM Y)

    UNION

    (SELECT B AS valor FROM X
     INTERSECT
     SELECT B FROM Y)

    UNION

    (SELECT C AS valor FROM X
     INTERSECT
     SELECT C FROM Y)

    UNION

    (SELECT D AS valor FROM X
     INTERSECT
     SELECT D FROM Y)
);

-- Pregunta 3, Diferencia de X y Z

SELECT C valor FROM X
MINUS
SELECT C FROM Z;
     
SELECT DISTINCT valor
FROM (
    SELECT C AS valor FROM Z
    UNION
    SELECT D FROM Z
);

